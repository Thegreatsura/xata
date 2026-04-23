package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"k8s.io/utils/ptr"

	"xata/internal/signoz"
)

var sigNozMetricName = map[string]struct {
	name, unit, metricType, temporalAgg, spaceAgg string
	additionalFilters                             map[string]string
}{
	// Maps Xata API metric names to SigNoz metric names
	"cpu":                  {name: "container.cpu.usage", unit: "percentage", metricType: "gauge", spaceAgg: "avg"},
	"memory":               {name: "container.memory.working_set", unit: "bytes", metricType: "gauge", spaceAgg: "avg"},
	"disk":                 {name: "cnpg_pg_database_size_bytes", unit: "bytes", metricType: "gauge", spaceAgg: "sum"},
	"connections_active":   {name: "cnpg_pg_stat_activity_connections_active", unit: "connections", metricType: "gauge", spaceAgg: "sum"},
	"connections_idle":     {name: "cnpg_pg_stat_activity_connections_idle", unit: "connections", metricType: "gauge", spaceAgg: "sum"},
	"network_ingress":      {name: "k8s.pod.network.io", unit: "bytes", metricType: "counter", temporalAgg: "increase", additionalFilters: map[string]string{"direction": "receive"}},
	"network_egress":       {name: "k8s.pod.network.io", unit: "bytes", metricType: "counter", temporalAgg: "increase", additionalFilters: map[string]string{"direction": "transmit"}},
	"iops_read":            {name: "cnpg_pg_stat_io_total_reads", unit: "iops", metricType: "counter", temporalAgg: "rate"},
	"iops_write":           {name: "cnpg_pg_stat_io_total_writes", unit: "iops", metricType: "counter", temporalAgg: "rate"},
	"latency_read":         {name: "cnpg_pg_stat_io_total_read_time_ms", unit: "ms", metricType: "counter", temporalAgg: "rate"},
	"latency_write":        {name: "cnpg_pg_stat_io_total_write_time_ms", unit: "ms", metricType: "counter", temporalAgg: "rate"},
	"throughput_read":      {name: "container_fs_reads_bytes_total", unit: "bytes", metricType: "counter", temporalAgg: "rate"},
	"throughput_write":     {name: "container_fs_writes_bytes_total", unit: "bytes", metricType: "counter", temporalAgg: "rate"},
	"wal_sync_time":        {name: "cnpg_collector_wal_sync_time", unit: "ms", metricType: "gauge", spaceAgg: "avg"},
	"replication_lag_time": {name: "cnpg_pg_replication_lag", unit: "s", metricType: "gauge", spaceAgg: "avg"},
}

type SigNozClient struct {
	client            *signoz.ClientWithResponses
	clustersNamespace string
}

// NewSigNozClient creates a new SigNoz client
func NewSigNozClient(endpoint, apiKey, clustersNamespace string) (*SigNozClient, error) {
	client, err := signoz.NewClientWithResponses(
		endpoint,
		signoz.WithRequestEditorFn(func(ctx context.Context, req *http.Request) error {
			req.Header.Set("SigNoz-Api-Key", apiKey)
			return nil
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create signoz client: %w", err)
	}

	return &SigNozClient{
		client:            client,
		clustersNamespace: clustersNamespace,
	}, nil
}

func (sc *SigNozClient) GetMetric(ctx context.Context, start, end time.Time, metric string, instances, aggregations []string) (*BranchMetrics, error) {
	if _, exists := sigNozMetricName[metric]; !exists {
		return nil, fmt.Errorf("metric %s not found", metric)
	}

	// Build request
	reqBody, queryToAgg, err := buildMetricsReq(sc.clustersNamespace, start, end, metric, instances, aggregations)
	if err != nil {
		return nil, err
	}

	response, err := sc.client.QueryRangeV5WithResponse(ctx, reqBody)
	if err != nil {
		return nil, fmt.Errorf("query range v5: %w", err)
	}

	if response.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", response.StatusCode())
	}

	if response.JSON200 == nil {
		return nil, fmt.Errorf("empty response")
	}

	if response.JSON200.Status != "success" {
		return nil, fmt.Errorf("unexpected status: %s", response.JSON200.Status)
	}

	// Parse response
	branchMetrics := BranchMetrics{
		Start:  start,
		End:    end,
		Metric: metric,
		Unit:   sigNozMetricName[metric].unit,
		Series: []MetricSeries{},
	}

	queryData := response.JSON200.Data.Data
	if queryData == nil || queryData.Results == nil {
		return &branchMetrics, nil
	}

	series, err := parseQueryResults(*queryData.Results, queryToAgg)
	if err != nil {
		return nil, err
	}
	branchMetrics.Series = series

	return &branchMetrics, nil
}

// parseQueryResults extracts metric series from SigNoz query results
func parseQueryResults(results []any, queryToAgg map[string]string) ([]MetricSeries, error) {
	data, err := json.Marshal(results)
	if err != nil {
		return nil, fmt.Errorf("marshal results: %w", err)
	}

	var parsedResults []signoz.Querybuildertypesv5TimeSeriesData
	if err := json.Unmarshal(data, &parsedResults); err != nil {
		return nil, fmt.Errorf("unmarshal time series data: %w", err)
	}

	series := make([]MetricSeries, 0)
	for _, result := range parsedResults {
		resultSeries, err := parseResult(result, queryToAgg)
		if err != nil {
			return nil, err
		}
		series = append(series, resultSeries...)
	}

	return series, nil
}

// parseResult extracts metric series from a single query result
func parseResult(result signoz.Querybuildertypesv5TimeSeriesData, queryToAgg map[string]string) ([]MetricSeries, error) {
	queryName := ptr.Deref(result.QueryName, "")
	agg, ok := queryToAgg[queryName]
	if !ok {
		return nil, fmt.Errorf("unexpected query name: %s", queryName)
	}

	if result.Aggregations == nil {
		return nil, nil
	}

	series := make([]MetricSeries, 0)
	for _, bucket := range *result.Aggregations {
		if bucket.Series == nil {
			continue
		}
		for _, s := range *bucket.Series {
			series = append(series, parseSeries(s, agg))
		}
	}

	return series, nil
}

// parseSeries converts a SigNoz series to a MetricSeries
func parseSeries(series signoz.Querybuildertypesv5TimeSeries, aggregation string) MetricSeries {
	return MetricSeries{
		Aggregation: aggregation,
		InstanceID:  extractInstanceID(series.Labels),
		Values:      parseValues(series.Values),
	}
}

// extractInstanceID retrieves the pod name from series labels
func extractInstanceID(labels *[]signoz.Querybuildertypesv5Label) string {
	if labels == nil {
		return ""
	}
	for _, label := range *labels {
		if label.Key == nil || label.Key.Name != "k8s.pod.name" || label.Value == nil {
			continue
		}
		if name, ok := (*label.Value).(string); ok {
			return name
		}
	}
	return ""
}

// parseValues converts SigNoz points to metric values
func parseValues(points *[]signoz.Querybuildertypesv5TimeSeriesValue) []Values {
	if points == nil {
		return nil
	}

	values := make([]Values, 0, len(*points))
	for _, point := range *points {
		if point.Timestamp == nil || point.Value == nil {
			continue
		}
		values = append(values, Values{
			Timestamp: time.UnixMilli(*point.Timestamp),
			Value:     float32(*point.Value),
		})
	}

	return values
}

// calculateStep determines the step interval based on the time difference between start and end.
func calculateStep(start, end time.Time) int {
	diff := end.Sub(start)
	switch {
	case diff < 24*time.Hour:
		return int(time.Minute.Seconds()) // Less than a day, use 1 minute step
	case diff < 3*24*time.Hour:
		return int((15 * time.Minute).Seconds()) // Less than 3 days, use 15 minutes step
	case diff < 7*24*time.Hour:
		return int((30 * time.Minute).Seconds()) // Less than a week, use 30 minutes step
	case diff < 30*24*time.Hour:
		return int((time.Hour).Seconds()) // Less than a month, use 1 hour step
	default: // For longer periods, use 6 hours step
		return int(6 * time.Hour.Seconds())
	}
}

func buildMetricsReq(clustersNamespace string, start, end time.Time, metricName string, instances, aggregations []string) (signoz.QueryRangeV5JSONRequestBody, map[string]string, error) {
	step := calculateStep(start, end)
	filterExpr := buildFilterExpression(clustersNamespace, instances, metricName)

	queries, queryToAgg, err := buildQueries(metricName, step, aggregations, filterExpr)
	if err != nil {
		return signoz.QueryRangeV5JSONRequestBody{}, nil, err
	}

	return signoz.QueryRangeV5JSONRequestBody{
		Start: new(int(start.UnixMilli())),
		End:   new(int(end.UnixMilli())),
		CompositeQuery: &signoz.Querybuildertypesv5CompositeQuery{
			Queries: &queries,
		},
		RequestType:   new(signoz.TimeSeries),
		SchemaVersion: new("v1"),
	}, queryToAgg, nil
}

func buildFilterExpression(namespace string, instances []string, metricName string) string {
	parts := make([]string, 0, 3)
	quoted := make([]string, len(instances))
	for i, inst := range instances {
		quoted[i] = escapeFilterString(inst)
	}
	parts = append(parts, "k8s.pod.name IN ["+strings.Join(quoted, ", ")+"]")
	parts = append(parts, "k8s.namespace.name = "+escapeFilterString(namespace))
	if info := sigNozMetricName[metricName]; info.additionalFilters != nil {
		for key, val := range info.additionalFilters {
			parts = append(parts, key+" = "+escapeFilterString(val))
		}
	}
	return strings.Join(parts, " AND ")
}

func escapeFilterString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

func buildQueries(metricName string, step int, aggregations []string, filterExpr string) ([]signoz.Querybuildertypesv5QueryEnvelope, map[string]string, error) {
	info := sigNozMetricName[metricName]
	queries := make([]signoz.Querybuildertypesv5QueryEnvelope, 0, len(aggregations))
	queryToAgg := make(map[string]string, len(aggregations))

	stepInterval := &signoz.Querybuildertypesv5Step{}
	if err := stepInterval.FromQuerybuildertypesv5Step1(float32(step)); err != nil {
		return nil, nil, fmt.Errorf("encode step interval: %w", err)
	}

	for i, agg := range aggregations {
		var timeAgg, spaceAgg string
		if info.metricType == "counter" {
			timeAgg = info.temporalAgg
			spaceAgg = agg
		} else {
			timeAgg = agg
			spaceAgg = info.spaceAgg
		}

		// Queries are named A, B, C, etc. in order to be able to interpret the response properly
		queryName := string(rune(65 + i))
		queryToAgg[queryName] = agg

		spec := signoz.Querybuildertypesv5QueryBuilderQueryGithubComSigNozSignozPkgTypesQuerybuildertypesQuerybuildertypesv5MetricAggregation{
			Name:   &queryName,
			Signal: new(signoz.Metrics),
			Aggregations: &[]signoz.Querybuildertypesv5MetricAggregation{
				{
					MetricName:       new(info.name),
					TimeAggregation:  new(signoz.MetrictypesTimeAggregation(timeAgg)),
					SpaceAggregation: new(signoz.MetrictypesSpaceAggregation(spaceAgg)),
				},
			},
			Filter: &signoz.Querybuildertypesv5Filter{
				Expression: &filterExpr,
			},
			GroupBy: &[]signoz.Querybuildertypesv5GroupByKey{
				{Name: "k8s.pod.name", FieldContext: new(signoz.Resource)},
			},
			StepInterval: stepInterval,
			Legend:       new("{{k8s.pod.name}}"),
			Disabled:     new(false),
		}

		envelope := signoz.Querybuildertypesv5QueryEnvelope{}
		if err := envelope.FromQuerybuildertypesv5QueryEnvelopeBuilderMetric(signoz.Querybuildertypesv5QueryEnvelopeBuilderMetric{
			Type: new(signoz.BuilderQuery),
			Spec: &spec,
		}); err != nil {
			return nil, nil, fmt.Errorf("encode query envelope: %w", err)
		}
		queries = append(queries, envelope)
	}

	return queries, queryToAgg, nil
}
