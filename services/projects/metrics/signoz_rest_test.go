package metrics

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"xata/internal/signoz"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	k8sNamespace = "xata-clusters"
	apiKey       = "test-api-key"
)

func TestGetMetric(t *testing.T) {
	tests := []struct {
		name           string
		metric         string
		startTime      time.Time
		endTime        time.Time
		instances      []string
		aggregations   []string
		unit           string
		mockStatusCode int
		mockResponse   *signoz.QueryRangeResponse
		expectError    bool
		assertRequest  func(t *testing.T, req *signoz.QueryRangeParams)
	}{
		{
			name:           "AVG CPU on single instance",
			metric:         "cpu",
			startTime:      time.UnixMilli(1715000000000),
			endTime:        time.UnixMilli(1715010000000),
			instances:      []string{"pod-1"},
			aggregations:   []string{"avg"},
			unit:           "percentage",
			mockStatusCode: 200,
			mockResponse: &signoz.QueryRangeResponse{
				ResultType: new("timeseries"),
				Result: &[]signoz.Result{
					{
						QueryName: new("A"),
						Series: &[]signoz.Series{
							{
								Labels: &map[string]string{"k8s.pod.name": "pod-1"},
								LabelsArray: &[]map[string]string{
									{"k8s.pod.name": "pod-1"},
								},
								Values: &[]signoz.Point{
									{
										Timestamp: 1715000000000,
										Value:     "42.5",
									},
								},
							},
						},
					},
				},
			},
			expectError: false,
			assertRequest: func(t *testing.T, req *signoz.QueryRangeParams) {
				require.NotNil(t, req.Variables)
				assert.Equal(t, k8sNamespace, (*req.Variables)["k8s_namespace_name"])
				// Variables are decoded as interface{} arrays from JSON
				podNames, ok := (*req.Variables)["k8s_pod_name"].([]any)
				require.True(t, ok)
				assert.Equal(t, 1, len(podNames))
				assert.Equal(t, "pod-1", podNames[0])
			},
		},
		{
			name:           "MIN, MAX CPU on 2 instances",
			metric:         "cpu",
			startTime:      time.UnixMilli(1715000000000),
			endTime:        time.UnixMilli(1715010000000),
			instances:      []string{"pod-1", "pod-2"},
			aggregations:   []string{"min", "max"},
			unit:           "percentage",
			mockStatusCode: 200,
			mockResponse: &signoz.QueryRangeResponse{
				ResultType: new(""),
				Result: &[]signoz.Result{
					{
						QueryName: new("B"),
						Series: &[]signoz.Series{
							{
								Labels: &map[string]string{"k8s.pod.name": "pod-1"},
								LabelsArray: &[]map[string]string{
									{"k8s.pod.name": "pod-1"},
								},
								Values: &[]signoz.Point{
									{Timestamp: 1746776340000, Value: "0.004387936"},
									{Timestamp: 1746776400000, Value: "0.004595945"},
								},
							},
							{
								Labels: &map[string]string{"k8s.pod.name": "pod-2"},
								LabelsArray: &[]map[string]string{
									{"k8s.pod.name": "pod-2"},
								},
								Values: &[]signoz.Point{
									{Timestamp: 1746776340000, Value: "0.0029397"},
									{Timestamp: 1746776400000, Value: "0.002136112"},
								},
							},
						},
					},
					{
						QueryName: new("A"),
						Series: &[]signoz.Series{
							{
								Labels: &map[string]string{"k8s.pod.name": "pod-1"},
								LabelsArray: &[]map[string]string{
									{"k8s.pod.name": "pod-1"},
								},
								Values: &[]signoz.Point{
									{Timestamp: 1746776340000, Value: "0.004387936"},
									{Timestamp: 1746776400000, Value: "0.004595945"},
								},
							},
							{
								Labels: &map[string]string{"k8s.pod.name": "pod-2"},
								LabelsArray: &[]map[string]string{
									{"k8s.pod.name": "pod-2"},
								},
								Values: &[]signoz.Point{
									{Timestamp: 1746776340000, Value: "0.0029397"},
									{Timestamp: 1746776400000, Value: "0.002136112"},
								},
							},
						},
					},
				},
			},
			expectError: false,
			assertRequest: func(t *testing.T, req *signoz.QueryRangeParams) {
				require.NotNil(t, req.Variables)
				assert.Equal(t, k8sNamespace, (*req.Variables)["k8s_namespace_name"])
				// Variables are decoded as interface{} arrays from JSON
				podNames, ok := (*req.Variables)["k8s_pod_name"].([]any)
				require.True(t, ok)
				assert.Equal(t, 2, len(podNames))
				assert.Equal(t, "pod-1", podNames[0])
				assert.Equal(t, "pod-2", podNames[1])
			},
		},
		{
			name:          "unknown metric name",
			metric:        "invalid_metric",
			instances:     []string{"pod-1"},
			aggregations:  []string{"avg"},
			expectError:   true,
			assertRequest: nil,
		},
		{
			name:           "Empty data response doesn't return nil series",
			metric:         "cpu",
			startTime:      time.UnixMilli(1715000000000),
			endTime:        time.UnixMilli(1715010000000),
			instances:      []string{"pod-1"},
			aggregations:   []string{"avg"},
			unit:           "percentage",
			mockStatusCode: 200,
			mockResponse: &signoz.QueryRangeResponse{
				ResultType: new(""),
				Result:     &[]signoz.Result{},
			},
			expectError: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedReq *signoz.QueryRangeParams

			// Create a test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify path
				assert.Equal(t, "/api/v4/query_range", r.URL.Path)

				// Verify API key header
				assert.Equal(t, apiKey, r.Header.Get("SIGNOZ-API-KEY"))

				// Capture and decode request
				if tt.mockResponse != nil {
					var req signoz.QueryRangeParams
					err := json.NewDecoder(r.Body).Decode(&req)
					require.NoError(t, err)
					capturedReq = &req
				}

				// Return mock response
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.mockStatusCode)
				if tt.mockResponse != nil {
					wrapper := map[string]any{
						"status": "success",
						"data":   tt.mockResponse,
					}
					err := json.NewEncoder(w).Encode(wrapper)
					require.NoError(t, err)
				}
			}))
			defer server.Close()

			client, err := NewSigNozClient(server.URL, apiKey, k8sNamespace)
			require.NoError(t, err)

			result, err := client.GetMetric(context.Background(), tt.startTime, tt.endTime, tt.metric, tt.instances, tt.aggregations)

			if tt.expectError {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.NotNil(t, result)
			assert.Equal(t, tt.metric, result.Metric)
			assert.Equal(t, tt.startTime, result.Start)
			assert.Equal(t, tt.endTime, result.End)
			assert.Equal(t, tt.unit, result.Unit)
			assert.NotNil(t, result.Series)
			if len(result.Series) > 0 {
				assert.Equal(t, len(tt.aggregations)*len(tt.instances), len(result.Series))
			}

			// Verify response data
			if tt.mockResponse != nil && tt.mockResponse.Result != nil {
				idx := 0
				for _, res := range *tt.mockResponse.Result {
					if res.Series != nil {
						for _, ser := range *res.Series {
							if ser.Labels != nil {
								assert.Equal(t, (*ser.Labels)["k8s.pod.name"], result.Series[idx].InstanceID)
							}
							if ser.Values != nil {
								for k, point := range *ser.Values {
									parsed, err := strconv.ParseFloat(point.Value, 32)
									assert.NoError(t, err)
									assert.InDelta(t, float32(parsed), result.Series[idx].Values[k].Value, 0.00001)
									assert.Equal(t, point.Timestamp, result.Series[idx].Values[k].Timestamp.UnixMilli())
								}
							}
							idx++
						}
					}
				}
			}

			if tt.assertRequest != nil && capturedReq != nil {
				tt.assertRequest(t, capturedReq)
			}
		})
	}
}
