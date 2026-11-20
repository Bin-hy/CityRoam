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
	"trippilot/internal/app"
	"trippilot/internal/config"
	"trippilot/internal/eval"
	"trippilot/internal/events"
	"trippilot/internal/logging"
	"trippilot/internal/plugins"
	"trippilot/internal/repo"
	"trippilot/internal/runstore"
	"trippilot/internal/skills"
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
	pluginCasesDir := flag.String("plugin-cases", "", "directory with plugin eval cases (default eval/cases-plugins when -plugins is set)")
	pluginsEnabled := flag.Bool("plugins", false, "enable hotel and guide research plugins from the environment")
	dryRun := flag.Bool("dry-run", false, "load cases and print the table header without calling the LLM")
	output := flag.String("output", "", "write the Markdown report to this file instead of stdout")
	flag.Parse()

	casesExplicit := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "cases" {
			casesExplicit = true
		}
	})
	if *pluginsEnabled {
		switch {
		case *pluginCasesDir != "":
			*casesDir = *pluginCasesDir
		case !casesExplicit:
			*casesDir = "eval/cases-plugins"
		}
	}
	if *pluginsEnabled && *output == "" {
		*output = "docs/eval-report-plugins.md"
	}

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
	notes := make([]string, 0, 3)
	nodeOptions := []agent.NodeOption{
		agent.WithSnapshotSource(snapshot),
		agent.WithPOILookup(snapshot),
		agent.WithRouteProvider(tools.NewRouteService(amap, "北京")),
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

	if *pluginsEnabled {
		wiring := app.WirePlugins(ctx, cfg, store, broker, logger)
		for _, status := range wiring.Statuses {
			if !status.Enabled {
				logger.Warn("plugin unavailable for eval", "plugin", status.Name, "reason", status.DisabledReason)
			}
		}
		for _, status := range wiring.Statuses {
			if !status.Enabled {
				return fmt.Errorf("plugin eval requires %s: %s", status.Name, status.DisabledReason)
			}
		}
		skillRegistry, err := skills.Load(cfg.SkillsDir)
		if err != nil {
			return fmt.Errorf("load skills: %w", err)
		}
		nodeOptions = append(nodeOptions,
			agent.WithToolLLM(llm),
			agent.WithResearchTools(wiring.Tools...),
			agent.WithResearchLimits(cfg.ResearchTimeout, cfg.ResearchMaxToolCalls),
			agent.WithResearchContext(plugins.WithRun),
			agent.WithSkillRegistry(skillRegistry),
		)
		notes = append(notes,
			fmt.Sprintf("技能：%d 个固定版本（目录 %s）", skillRegistry.Len(), cfg.SkillsDir),
			fmt.Sprintf("插件开关：HOTEL_PLUGIN_ENABLED=%t，GUIDE_PLUGIN_ENABLED=%t", cfg.HotelPluginEnabled, cfg.GuidePluginEnabled),
			fmt.Sprintf("酒店 sidecar：%s（healthz 通过）", cfg.HotelToolURL),
			fmt.Sprintf("攻略搜索：%s", cfg.GuideSearchBaseURL),
			fmt.Sprintf("研究时限：%s；研究工具调用上限：%d", cfg.ResearchTimeout, cfg.ResearchMaxToolCalls),
			fmt.Sprintf("命令：go run ./cmd/eval -plugins -cases %s -output %s", *casesDir, *output),
		)
	}

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
	report.Notes = append(report.Notes, notes...)
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
