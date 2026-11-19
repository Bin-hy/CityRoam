package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"trippilot/internal/agent"
	"trippilot/internal/domain"
	"trippilot/internal/tools"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "cli:", err)
		os.Exit(1)
	}
}

type noopEvents struct{}

func (noopEvents) Publish(context.Context, string, string, any) error { return nil }

func run() error {
	fakeLLM := flag.Bool("fake-llm", false, "use a deterministic local LLM")
	poiPath := flag.String("poi", "data/poi/beijing.json", "POI snapshot path")
	constraint := flag.String("constraint", "", "extra natural-language constraints")
	flag.Parse()

	parts := append([]string{strings.TrimSpace(flag.Arg(0))}, splitConstraints(*constraint)...)
	input := strings.TrimSpace(strings.Join(nonEmpty(parts), "，"))
	if input == "" {
		return errors.New(`usage: cli [--fake-llm] [--constraint "预算800元"] "北京两天，想看清宫和长城"`)
	}

	snapshot, err := tools.LoadSnapshot(*poiPath)
	if err != nil {
		return err
	}

	var llm agent.LLMClient
	if *fakeLLM {
		llm = fakeClient{startDate: time.Now().AddDate(0, 0, 1).Format("2006-01-02")}
	} else {
		client, err := realClient()
		if err != nil {
			return err
		}
		llm = client
	}

	amapKey := strings.TrimSpace(os.Getenv("AMAP_WEB_KEY"))
	var amap *tools.AMapClient
	if amapKey != "" {
		amap = tools.NewAMapClient(amapKey)
	}

	options := []agent.NodeOption{
		agent.WithSnapshotSource(snapshot),
		agent.WithPOILookup(snapshot),
		agent.WithRouteProvider(tools.NewRouteService(amap, "beijing")),
	}
	if amap != nil {
		options = append(options, agent.WithExternalSource(tools.NewAMapSource(amap)))
	}
	nodes := agent.NewNodes(llm, noopEvents{}, options...)

	graph, err := agent.NewGraph(context.Background(), nodes)
	if err != nil {
		return err
	}
	state, err := graph.Run(context.Background(), &agent.TripState{
		RunID:  "cli",
		TripID: "cli",
		Mode:   "initial",
		Input:  input,
	})
	if err != nil {
		return err
	}

	return printResult(state)
}

func realClient() (*agent.OpenAIClient, error) {
	baseURL := strings.TrimSpace(os.Getenv("LLM_BASE_URL"))
	model := strings.TrimSpace(os.Getenv("LLM_MODEL"))
	if baseURL == "" || model == "" {
		return nil, errors.New("LLM_BASE_URL and LLM_MODEL are required (or pass --fake-llm)")
	}
	return agent.NewOpenAIClient(baseURL, os.Getenv("LLM_API_KEY"), model)
}

type result struct {
	Status       string               `json:"status"`
	Plan         domain.Itinerary     `json:"plan"`
	Violations   []domain.Violation   `json:"violations"`
	Suggestions  []string             `json:"suggestions,omitempty"`
	StopReason   string               `json:"stop_reason,omitempty"`
	Missing      []string             `json:"missing,omitempty"`
	Explanations []domain.Explanation `json:"explanations,omitempty"`
}

func printResult(state *agent.TripState) error {
	payload := result{
		Status:       state.Status,
		Plan:         state.Plan,
		Violations:   append([]domain.Violation{}, state.Violations...),
		Suggestions:  state.Suggestions,
		StopReason:   state.StopReason,
		Missing:      state.Missing,
		Explanations: state.Draft.Explanations,
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(payload)
}

func splitConstraints(raw string) []string {
	return strings.Split(raw, "，")
}

func nonEmpty(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

type fakeClient struct {
	startDate string
}

func (f fakeClient) GenerateJSON(_ context.Context, _ string, schema json.RawMessage) (json.RawMessage, agent.Usage, error) {
	switch {
	case bytes.Equal(schema, agent.ConstraintSchema):
		return json.RawMessage(`{"city":"beijing","days":2,"start_date":"` + f.startDate + `","missing":[]}`), agent.Usage{}, nil
	case bytes.Equal(schema, agent.DraftPlanSchema):
		return draftReply(f.startDate), agent.Usage{}, nil
	case bytes.Equal(schema, agent.DayPatchSchema):
		return json.RawMessage(`{"day_index":0,"items":[{"poi_id":"jingshan-park","type":"poi","duration_min":60}],"reason":"保持原方案"}`), agent.Usage{}, nil
	default:
		return nil, agent.Usage{}, fmt.Errorf("fake LLM does not handle schema: %s", schema)
	}
}

func draftReply(startDate string) json.RawMessage {
	start, err := time.Parse("2006-01-02", startDate)
	if err != nil {
		start = time.Now()
	}
	payload, _ := json.Marshal(domain.DraftPlan{
		Days: []domain.DraftDay{
			{Date: start.Format("2006-01-02"), Items: []domain.DraftItem{{POIID: "jingshan-park", Type: domain.ItemTypePOI, DurationMin: 60}}},
			{Date: start.AddDate(0, 0, 1).Format("2006-01-02"), Items: []domain.DraftItem{{POIID: "beihai-park", Type: domain.ItemTypePOI, DurationMin: 60}}},
		},
		Explanations: []domain.Explanation{
			{POIID: "jingshan-park", Text: "登高俯瞰故宫中轴线"},
			{POIID: "beihai-park", Text: "沿湖散步，节奏轻松"},
		},
	})
	return payload
}
