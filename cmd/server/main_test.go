package main

import (
	"testing"

	"github.com/pj-hoakari/tolo-graph-authoring/internal/domain"
)

func TestGraphSizeLimitFromEnv(t *testing.T) {
	for _, tc := range []struct {
		name    string
		nodes   string
		edges   string
		want    domain.GraphSizeLimit
		wantErr bool
	}{
		{"defaults when unset", "", "", domain.DefaultGraphSizeLimit, false},
		{"configured values", "60", "150", domain.GraphSizeLimit{Points: 60, Routes: 150}, false},
		{"zero nodes", "0", "", domain.GraphSizeLimit{}, true},
		{"negative edges", "", "-1", domain.GraphSizeLimit{}, true},
		{"not a number", "many", "", domain.GraphSizeLimit{}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("TOLO_GRAPH_MAX_NODES", tc.nodes)
			t.Setenv("TOLO_GRAPH_MAX_EDGES", tc.edges)

			got, err := graphSizeLimitFromEnv()
			if (err != nil) != tc.wantErr {
				t.Fatalf("graphSizeLimitFromEnv() error = %v, wantErr %v", err, tc.wantErr)
			}

			if got != tc.want {
				t.Errorf("graphSizeLimitFromEnv() = %+v, want %+v", got, tc.want)
			}
		})
	}
}
