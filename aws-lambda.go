package main

import (
	"errors"
	"flag"
	"log"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
	mp "github.com/mackerelio/go-mackerel-plugin-helper"
)

const (
	namespace          = "AWS/Lambda"
	metricsTypeAverage = "Average"
	metricsTypeSum     = "Sum"
	metricsTypeMaximum = "Maximum"
	metricsTypeMinimum = "Minimum"
)

type metrics struct {
	CloudWatchName string
	MetricTypes    []metricType
	MackerelName   string
	Type           string
}

type metricType struct {
	MackerelName string
	Type         string
}

// LambdaPlugin mackerel plugin for aws kinesis
type LambdaPlugin struct {
	Name   string
	Prefix string

	// AccessKeyID     string
	// SecretAccessKey string
	Region     string
	CloudWatch *cloudwatch.CloudWatch
}

// MetricKeyPrefix interface for PluginWithPrefix
func (p LambdaPlugin) MetricKeyPrefix() string {
	if p.Prefix == "" {
		p.Prefix = "lambda"
	}
	return p.Prefix
}

// prepare creates CloudWatch instance
func (p *LambdaPlugin) prepare() error {
	sess, err := session.NewSession()
	if err != nil {
		return err
	}
	if p.Region != "" {
		p.CloudWatch = cloudwatch.New(sess, aws.NewConfig().WithRegion(p.Region))
	} else {
		p.CloudWatch = cloudwatch.New(sess)
	}

	return nil
}

// getLastPoint fetches a CloudWatch metric and parse
func (p LambdaPlugin) getLastPoint(metric metrics) (*cloudwatch.Datapoint, error) {
	now := time.Now()
	statsInput := make([]*string, len(metric.MetricTypes))
	for i, typ := range metric.MetricTypes {
		statsInput[i] = aws.String(typ.Type)
	}
	response, err := p.CloudWatch.GetMetricStatistics(&cloudwatch.GetMetricStatisticsInput{
		Dimensions: []*cloudwatch.Dimension{
			{
				Name:  aws.String("FunctionName"),
				Value: aws.String(p.Name),
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

// FetchMetrics fetch the metrics
func (p LambdaPlugin) FetchMetrics() (map[string]interface{}, error) {
	stat := make(map[string]interface{})

	for _, met := range [...]metrics{
		{CloudWatchName: "Invocations", MetricTypes: []metricType{{MackerelName: "invocations_total", Type: metricsTypeSum}}},
		{CloudWatchName: "Errors", MetricTypes: []metricType{{MackerelName: "invocations_error", Type: metricsTypeSum}}},
		{CloudWatchName: "Throttles", MetricTypes: []metricType{{MackerelName: "invocations_throttles", Type: metricsTypeSum}}},
		{CloudWatchName: "Duration", MetricTypes: []metricType{
			{MackerelName: "duration_avg", Type: metricsTypeAverage},
			{MackerelName: "duration_max", Type: metricsTypeMaximum},
			{MackerelName: "duration_min", Type: metricsTypeMinimum},
		}},
	} {
		v, err := p.getLastPoint(met)
		if err == nil {
			for _, typ := range met.MetricTypes {
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
	return stat, nil
}

// GraphDefinition of LambdaPlugin
func (p LambdaPlugin) GraphDefinition() map[string]mp.Graphs {
	labelPrefix := strings.Title(p.Prefix)
	labelPrefix = strings.Replace(labelPrefix, "-", " ", -1)

	var graphdef = map[string]mp.Graphs{
		"invocations": {
			Label: (labelPrefix + " Invocations"),
			Unit:  "integer",
			Metrics: []mp.Metrics{
				{Name: "invocations_total", Label: "Total"},
				{Name: "invocations_error", Label: "Error"},
				{Name: "invocations_throttles", Label: "Throttles"},
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

func main() {
	// optAccessKeyID := flag.String("access-key-id", "", "AWS Access Key ID")
	// optSecretAccessKey := flag.String("secret-access-key", "", "AWS Secret Access Key")
	optRegion := flag.String("region", "", "AWS Region")
	optIdentifier := flag.String("identifier", "", "Stream Name")
	optTempfile := flag.String("tempfile", "", "Temp file name")
	optPrefix := flag.String("metric-key-prefix", "lambda", "Metric key prefix")
	flag.Parse()

	var plugin LambdaPlugin

	// plugin.AccessKeyID = *optAccessKeyID
	// plugin.SecretAccessKey = *optSecretAccessKey
	plugin.Region = *optRegion
	plugin.Name = *optIdentifier
	plugin.Prefix = *optPrefix

	err := plugin.prepare()
	if err != nil {
		log.Fatalln(err)
	}

	helper := mp.NewMackerelPlugin(plugin)
	helper.Tempfile = *optTempfile

	helper.Run()
}
