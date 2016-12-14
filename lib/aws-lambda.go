package mpawslambda

import (
	"errors"
	"flag"
	"log"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
	"github.com/aws/aws-sdk-go/service/cloudwatch/cloudwatchiface"
	mp "github.com/mackerelio/go-mackerel-plugin-helper"
)

const (
	namespace          = "AWS/Lambda"
	metricsTypeAverage = "Average"
	metricsTypeSum     = "Sum"
	metricsTypeMaximum = "Maximum"
	metricsTypeMinimum = "Minimum"
)

// has 1 CloudWatch MetricName and corresponding N Mackerel Metrics
type metricsGroup struct {
	CloudWatchName string
	Metrics        []metric
}

type metric struct {
	MackerelName string
	Type         string
}

// LambdaPlugin mackerel plugin for aws Lambda
type LambdaPlugin struct {
	FunctionName string
	Prefix       string

	AccessKeyID     string
	SecretAccessKey string
	Region          string

	CloudWatch *cloudwatch.CloudWatch
}

// MetricKeyPrefix interface for PluginWithPrefix
func (p LambdaPlugin) MetricKeyPrefix() string {
	return p.Prefix
}

// prepare creates CloudWatch instance
func (p *LambdaPlugin) prepare() error {

	sess, err := session.NewSession()
	if err != nil {
		return err
	}

	config := aws.NewConfig()
	if p.AccessKeyID != "" && p.SecretAccessKey != "" {
		config = config.WithCredentials(credentials.NewStaticCredentials(p.AccessKeyID, p.SecretAccessKey, ""))
	}
	if p.Region != "" {
		config = config.WithRegion(p.Region)
	}

	p.CloudWatch = cloudwatch.New(sess, config)

	return nil
}

// getLastPoint fetches a CloudWatch metric and parse
func getLastPointFromCloudWatch(cw cloudwatchiface.CloudWatchAPI, functionName string, metric metricsGroup) (*cloudwatch.Datapoint, error) {
	now := time.Now()
	statsInput := make([]*string, len(metric.Metrics))
	for i, typ := range metric.Metrics {
		statsInput[i] = aws.String(typ.Type)
	}
	response, err := cw.GetMetricStatistics(&cloudwatch.GetMetricStatisticsInput{
		Dimensions: []*cloudwatch.Dimension{
			{
				Name:  aws.String("FunctionName"),
				Value: aws.String(functionName),
			},
		},
		StartTime:  aws.Time(now.Add(time.Duration(180) * time.Second * -1)), // 3 min
		EndTime:    aws.Time(now),
		MetricName: aws.String(metric.CloudWatchName),
		Period:     aws.Int64(600),
		Statistics: statsInput,
		Namespace:  aws.String(namespace),
	})
	if err != nil {
		return nil, err
	}

	datapoints := response.Datapoints
	if len(datapoints) == 0 {
		return nil, errors.New("fetched no datapoints")
	}

	latest := new(time.Time)
	var latestDp *cloudwatch.Datapoint
	for _, dp := range datapoints {
		if dp.Timestamp.Before(*latest) {
			continue
		}

		latest = dp.Timestamp
		latestDp = dp
	}

	return latestDp, nil
}

// TransformMetrics converts some of datapoints to post differences of two metrics
func (p LambdaPlugin) TransformMetrics(stats map[string]interface{}) map[string]interface{} {
	// Although stats are interface{}, those values from cloudwatch.Datapoint are guaranteed to be float64.
	if totalCount, ok := stats["TEMPORARY_invocations_total"].(float64); ok {
		if errorCount, ok := stats["invocations_error"].(float64); ok {
			stats["invocations_success"] = totalCount - errorCount
		} else {
			stats["invocations_success"] = totalCount
		}
		delete(stats, "TEMPORARY_invocations_total")
	}
	return stats
}

// FetchMetrics fetch the metrics
func (p LambdaPlugin) FetchMetrics() (map[string]interface{}, error) {
	stat := make(map[string]interface{})

	for _, met := range [...]metricsGroup{
		{CloudWatchName: "Invocations", Metrics: []metric{
			{MackerelName: "TEMPORARY_invocations_total", Type: metricsTypeSum},
		}},
		{CloudWatchName: "Errors", Metrics: []metric{
			{MackerelName: "invocations_error", Type: metricsTypeSum},
		}},
		{CloudWatchName: "Throttles", Metrics: []metric{
			{MackerelName: "invocations_throttles", Type: metricsTypeSum},
		}},
		{CloudWatchName: "Duration", Metrics: []metric{
			{MackerelName: "duration_avg", Type: metricsTypeAverage},
			{MackerelName: "duration_max", Type: metricsTypeMaximum},
			{MackerelName: "duration_min", Type: metricsTypeMinimum},
		}},
	} {
		v, err := getLastPointFromCloudWatch(p.CloudWatch, p.FunctionName, met)
		if err == nil {
			for _, typ := range met.Metrics {
				switch typ.Type {
				case metricsTypeAverage:
					stat[typ.MackerelName] = *v.Average
				case metricsTypeSum:
					stat[typ.MackerelName] = *v.Sum
				case metricsTypeMaximum:
					stat[typ.MackerelName] = *v.Maximum
				case metricsTypeMinimum:
					stat[typ.MackerelName] = *v.Minimum
				}
			}
		} else {
			log.Printf("%s: %s", met, err)
		}
	}
	return p.TransformMetrics(stat), nil
}

// GraphDefinition of LambdaPlugin
func (p LambdaPlugin) GraphDefinition() map[string]mp.Graphs {
	labelPrefix := strings.Title(p.Prefix)

	var graphdef = map[string]mp.Graphs{
		"invocations": {
			Label: (labelPrefix + " Invocations"),
			Unit:  "integer",
			Metrics: []mp.Metrics{
				{Name: "invocations_success", Label: "Success", Stacked: true},
				{Name: "invocations_error", Label: "Error", Stacked: true},
				{Name: "invocations_throttles", Label: "Throttles", Stacked: true},
			},
		},
		"duration": {
			Label: (labelPrefix + " Duration"),
			Unit:  "float",
			Metrics: []mp.Metrics{
				{Name: "duration_avg", Label: "Average"},
				{Name: "duration_max", Label: "Maximum"},
				{Name: "duration_min", Label: "Minimum"},
			},
		},
	}
	return graphdef
}

// Do the plugin
func Do() {
	optAccessKeyID := flag.String("access-key-id", "", "AWS Access Key ID")
	optSecretAccessKey := flag.String("secret-access-key", "", "AWS Secret Access Key")
	optRegion := flag.String("region", "", "AWS Region")
	optFunctionName := flag.String("function-name", "", "Function Name")
	optTempfile := flag.String("tempfile", "", "Temp file name")
	optPrefix := flag.String("metric-key-prefix", "lambda", "Metric key prefix")
	flag.Parse()

	var plugin LambdaPlugin

	plugin.AccessKeyID = *optAccessKeyID
	plugin.SecretAccessKey = *optSecretAccessKey
	plugin.Region = *optRegion

	plugin.FunctionName = *optFunctionName
	plugin.Prefix = *optPrefix

	err := plugin.prepare()
	if err != nil {
		log.Fatalln(err)
	}

	helper := mp.NewMackerelPlugin(plugin)
	helper.Tempfile = *optTempfile

	helper.Run()
}
