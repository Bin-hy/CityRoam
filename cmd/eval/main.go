package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"trippilot/internal/agent"
	"trippilot/internal/config"
	"trippilot/internal/eval"
	"trippilot/internal/events"
	"trippilot/internal/logging"
	"trippilot/internal/repo"
	"trippilot/internal/runstore"
	"trippilot/internal/tools"
	"trippilot/migrations"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	casesDir := flag.String("cases", "eval/cases", "directory with eval case JSON files")
	dryRun := flag.Bool("dry-run", false, "load cases and print the table header without calling the LLM")
	output := flag.String("output", "", "write the Markdown report to this file instead of stdout")
	flag.Parse()

	cases, err := eval.LoadCases(*casesDir)
	if err != nil {
		return err
	}
	if *dryRun {
		fmt.Print(eval.DryRunMarkdown(cases))
		return nil
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.LLMBaseURL == "" || cfg.LLMModel == "" {
		return errors.New("LLM_BASE_URL and LLM_MODEL are required")
	}
	logger := logging.New(cfg.LogLevel)

	db, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}
	if err := migrations.Up(db); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}

	store, err := repo.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer store.Close()

	snapshot, err := tools.LoadEmbeddedSnapshot()
	if err != nil {
		return err
	}

	llm, err := agent.NewOpenAIClient(cfg.LLMBaseURL, cfg.LLMAPIKey, cfg.LLMModel,
		agent.WithLLMLogger(logger))
	if err != nil {
		return err
	}

	var amap *tools.AMapClient
	if cfg.AMapWebKey != "" {
		amap = tools.NewAMapClient(cfg.AMapWebKey)
	}
	nodeOptions := []agent.NodeOption{
		agent.WithSnapshotSource(snapshot),
		agent.WithPOILookup(snapshot),
		agent.WithRouteProvider(tools.NewRouteService(amap, "beijing")),
		agent.WithNodeLogger(logger),
	}
	if amap != nil {
		nodeOptions = append(nodeOptions, agent.WithExternalSource(tools.NewAMapSource(amap)))
	}

	broker, err := events.NewBroker(events.NewRepoStore(store), events.WithLogger(logger))
	if err != nil {
		return err
	}
	defer broker.Close()

	nodes := agent.NewNodes(llm, broker, nodeOptions...)
	runner, err := agent.NewRunner(ctx, runstore.New(store), broker, nodes,
		agent.WithRunnerConcurrency(1),
		agent.WithRunnerLogger(logger),
	)
	if err != nil {
		return err
	}

	batch := eval.NewRunner(store, runner,
		eval.WithRunnerLogger(logger),
		eval.WithCaseTimeout(3*time.Minute),
	)
	report, runErr := batch.Run(ctx, cases)
	runner.Wait()

	markdown := eval.Markdown(report)
	if *output != "" {
		if err := os.WriteFile(*output, []byte(markdown), 0o644); err != nil {
			return fmt.Errorf("write report: %w", err)
		}
		fmt.Fprintf(os.Stderr, "report written to %s\n", *output)
	} else {
		fmt.Print(markdown)
	}

	if runErr != nil {
		return runErr
	}
	if report.Summary.Passed < report.Summary.Cases {
		logger.Warn("eval finished with failing cases",
			"passed", report.Summary.Passed,
			"cases", report.Summary.Cases,
		)
	}
	return nil
}
