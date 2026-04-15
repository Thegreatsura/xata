package metrics

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"k8s.io/utils/ptr"

	"xata/internal/signoz"
)

var sigNozMetricName = map[string]struct {
	name, unit, dataType, metricType, temporalAgg, spaceAgg string
	additionalFilters                                       map[string]string
}{
	// Maps Xata API metric names to SigNoz metric names
	"cpu":                  {name: "container.cpu.utilization", unit: "percentage", dataType: "float64", metricType: "gauge", spaceAgg: "avg"},
	"memory":               {name: "container.memory.working_set", unit: "bytes", dataType: "float64", metricType: "gauge", spaceAgg: "avg"},
	"disk":                 {name: "cnpg_pg_database_size_bytes", unit: "bytes", dataType: "float64", metricType: "gauge", spaceAgg: "sum"},
	"connections_active":   {name: "cnpg_pg_stat_activity_connections_active", unit: "connections", dataType: "float64", metricType: "gauge", spaceAgg: "sum"},
	"connections_idle":     {name: "cnpg_pg_stat_activity_connections_idle", unit: "connections", dataType: "float64", metricType: "gauge", spaceAgg: "sum"},
	"network_ingress":      {name: "k8s.pod.network.io", unit: "bytes", dataType: "float64", metricType: "counter", temporalAgg: "increase", additionalFilters: map[string]string{"direction": "receive"}},
	"network_egress":       {name: "k8s.pod.network.io", unit: "bytes", dataType: "float64", metricType: "counter", temporalAgg: "increase", additionalFilters: map[string]string{"direction": "transmit"}},
	"iops_read":            {name: "cnpg_pg_stat_io_total_reads", unit: "iops", dataType: "float64", metricType: "counter", temporalAgg: "rate"},
	"iops_write":           {name: "cnpg_pg_stat_io_total_writes", unit: "iops", dataType: "float64", metricType: "counter", temporalAgg: "rate"},
	"latency_read":         {name: "cnpg_pg_stat_io_total_read_time_ms", unit: "ms", dataType: "float64", metricType: "counter", temporalAgg: "rate"},
	"latency_write":        {name: "cnpg_pg_stat_io_total_write_time_ms", unit: "ms", dataType: "float64", metricType: "counter", temporalAgg: "rate"},
	"throughput_read":      {name: "container_fs_reads_bytes_total", unit: "bytes", dataType: "float64", metricType: "counter", temporalAgg: "rate"},
	"throughput_write":     {name: "container_fs_writes_bytes_total", unit: "bytes", dataType: "float64", metricType: "counter", temporalAgg: "rate"},
	"wal_sync_time":        {name: "cnpg_collector_wal_sync_time", unit: "ms", dataType: "float64", metricType: "gauge", spaceAgg: "avg"},
	"replication_lag_time": {name: "cnpg_pg_replication_lag", unit: "s", dataType: "float64", metricType: "gauge", spaceAgg: "avg"},
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
			req.Header.Set("SIGNOZ-API-KEY", apiKey)
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
	reqBody, queryToAgg := buildMetricsReq(sc.clustersNamespace, start, end, metric, instances, aggregations)

	response, err := sc.client.QueryRangeV4WithResponse(ctx, &signoz.QueryRangeV4Params{}, reqBody)
	if err != nil {
		return nil, fmt.Errorf("query range v4: %w", err)
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

	queryResp := response.JSON200.Data

	// Parse response
	branchMetrics := BranchMetrics{
		Start:  start,
		End:    end,
		Metric: metric,
		Unit:   sigNozMetricName[metric].unit,
	}

	if queryResp.Result == nil || len(*queryResp.Result) == 0 {
		branchMetrics.Series = []MetricSeries{}
		return &branchMetrics, nil
	}

	series, err := parseQueryResults(*queryResp.Result, queryToAgg)
	if err != nil {
		return nil, err
	}
	branchMetrics.Series = series

	return &branchMetrics, nil
}

// parseQueryResults extracts metric series from SigNoz query results
func parseQueryResults(results []signoz.Result, queryToAgg map[string]string) ([]MetricSeries, error) {
	// Count total series for efficient allocation
	totalSeries := 0
	for _, result := range results {
		if result.Series != nil {
			totalSeries += len(*result.Series)
		}
	}

	series := make([]MetricSeries, 0, totalSeries)

	for _, result := range results {
		resultSeries, err := parseResult(result, queryToAgg)
		if err != nil {
			return nil, err
		}
		series = append(series, resultSeries...)
	}

	return series, nil
}

// parseResult extracts metric series from a single query result
func parseResult(result signoz.Result, queryToAgg map[string]string) ([]MetricSeries, error) {
	queryName := ptr.Deref(result.QueryName, "")
	agg, ok := queryToAgg[queryName]
	if !ok {
		return nil, fmt.Errorf("unexpected query name: %s", queryName)
	}

	if result.Series == nil {
		return nil, nil
	}

	series := make([]MetricSeries, 0, len(*result.Series))
	for _, s := range *result.Series {
		metricSeries, err := parseSeries(s, agg)
		if err != nil {
			return nil, err
		}
		series = append(series, metricSeries)
	}

	return series, nil
}

// parseSeries converts a SigNoz series to a MetricSeries
func parseSeries(series signoz.Series, aggregation string) (MetricSeries, error) {
	metricSeries := MetricSeries{
		Aggregation: aggregation,
		InstanceID:  extractInstanceID(series.Labels),
	}

	values, err := parseValues(series.Values)
	if err != nil {
		return MetricSeries{}, err
	}
	metricSeries.Values = values

	return metricSeries, nil
}

// extractInstanceID retrieves the pod name from series labels
func extractInstanceID(labels *map[string]string) string {
	if labels == nil {
		return ""
	}
	return (*labels)["k8s.pod.name"]
}

// parseValues converts SigNoz points to metric values
func parseValues(points *[]signoz.Point) ([]Values, error) {
	if points == nil {
		return nil, nil
	}

	values := make([]Values, len(*points))
	for i, point := range *points {
		floatVal, err := strconv.ParseFloat(point.Value, 32)
		if err != nil {
			return nil, fmt.Errorf("parse value at index %d: %w", i, err)
		}
		values[i] = Values{
			Timestamp: time.UnixMilli(point.Timestamp),
			Value:     float32(floatVal),
		}
	}

	return values, nil
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

func buildMetricsReq(clustersNamespace string, start, end time.Time, metricName string, instances, aggregations []string) (signoz.QueryRangeParams, map[string]string) {
	step := calculateStep(start, end)
	queries, queryToAgg := buildQueries(metricName, step, aggregations)

	formatForWeb := false
	variables := map[string]any{
		"k8s_pod_name":       instances,
		"k8s_namespace_name": clustersNamespace,
	}
	fillGaps := false

	reqPayload := signoz.QueryRangeParams{
		Start:        start.UnixMilli(),
		End:          end.UnixMilli(),
		Step:         int64(step),
		FormatForWeb: &formatForWeb,
		Variables:    &variables,
		CompositeQuery: signoz.CompositeQuery{
			QueryType:      signoz.CompositeQueryQueryTypeBuilder,
			PanelType:      signoz.CompositeQueryPanelTypeGraph,
			FillGaps:       &fillGaps,
			BuilderQueries: &queries,
		},
	}

	return reqPayload, queryToAgg
}

func buildQueries(metricName string, step int, aggregations []string) (map[string]signoz.BuilderQuery, map[string]string) {
	queries := make(map[string]signoz.BuilderQuery)
	queryToAgg := make(map[string]string)

	for i, agg := range aggregations {
		timeAgg, spaceAgg := agg, sigNozMetricName[metricName].spaceAgg
		if sigNozMetricName[metricName].metricType == "counter" {
			timeAgg = sigNozMetricName[metricName].temporalAgg
			spaceAgg = agg
		}

		// Queries are named A, B, C, etc. in order to be able to interpret the response properly
		queryName := string(rune(65 + i))
		queryToAgg[queryName] = agg

		// Build aggregate attribute
		dataType := signoz.AttributeKeyDataType(sigNozMetricName[metricName].dataType)
		isColumn := true
		aggregateAttr := signoz.AttributeKey{
			Key:      sigNozMetricName[metricName].name,
			DataType: &dataType,
			IsColumn: &isColumn,
		}

		// Build filter items
		filterItems := []signoz.FilterItem{
			{
				Key: signoz.AttributeKey{
					Key:      "k8s.pod.name",
					DataType: ptr.To(signoz.AttributeKeyDataTypeString),
					Type:     new(any("tag")),
					IsColumn: new(false),
				},
				Op:    signoz.FilterItemOpIn,
				Value: new(any("{{.k8s_pod_name}}")),
			},
			{
				Key: signoz.AttributeKey{
					Key:      "k8s.namespace.name",
					DataType: ptr.To(signoz.AttributeKeyDataTypeString),
					Type:     new(any("tag")),
					IsColumn: new(false),
				},
				Op:    signoz.FilterItemOpEqual,
				Value: new(any("{{.k8s_namespace_name}}")),
			},
		}

		// Add additional filters if any
		if sigNozMetricName[metricName].additionalFilters != nil {
			for key, value := range sigNozMetricName[metricName].additionalFilters {
				filterItems = append(filterItems, signoz.FilterItem{
					Key: signoz.AttributeKey{
						Key:      key,
						DataType: ptr.To(signoz.AttributeKeyDataTypeString),
						Type:     new(any("tag")),
						IsColumn: new(false),
					},
					Op:    signoz.FilterItemOpEqual,
					Value: new(any(value)),
				})
			}
		}

		filters := signoz.FilterSet{
			Items: &filterItems,
		}

		// Build group by
		groupBy := []signoz.AttributeKey{
			{
				Key:      "k8s.pod.name",
				DataType: ptr.To(signoz.AttributeKeyDataTypeString),
				Type:     new(any("tag")),
				IsColumn: new(false),
			},
		}

		legend := "{{k8s.pod.name}}"
		disabled := false

		query := signoz.BuilderQuery{
			QueryName:          queryName,
			DataSource:         signoz.BuilderQueryDataSourceMetrics,
			AggregateAttribute: &aggregateAttr,
			Expression:         queryName,
			Disabled:           &disabled,
			TimeAggregation:    new(signoz.BuilderQueryTimeAggregation(timeAgg)),
			SpaceAggregation:   new(signoz.BuilderQuerySpaceAggregation(spaceAgg)),
			StepInterval:       int64(step),
			Filters:            &filters,
			GroupBy:            &groupBy,
			Legend:             &legend,
		}

		queries[queryName] = query
	}

	return queries, queryToAgg
}
