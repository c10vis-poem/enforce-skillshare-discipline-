package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"skillshare/internal/config"
	"skillshare/internal/skillserve"
	"skillshare/internal/ui"
)

const mcpServeUsage = "skillshare mcp serve [--target <name>] [--http <addr> | --check] [-g|-p]"

// mcpServeTokenEnv holds the bearer token HTTP clients must send.
const mcpServeTokenEnv = "SKILLSHARE_MCP_TOKEN"

// cmdMCPServe serves the managed skills over MCP (SEP-2640). It never writes:
// stdout carries the protocol, so diagnostics go to stderr.
func cmdMCPServe(args []string) error {
	mode, rest, err := parseModeArgs(args, "--target", "--http")
	if err != nil {
		return err
	}
	var target, addr string
	check := false
	for i := 0; i < len(rest); i++ {
		switch a := rest[i]; a {
		case "--check":
			check = true
		case "--target", "--http":
			if i+1 == len(rest) {
				return fmt.Errorf("%s requires a value", a)
			}
			i++
			if a == "--target" {
				target = rest[i]
			} else {
				addr = rest[i]
			}
		default:
			return fmt.Errorf("unknown mcp serve argument %q; usage: %s", a, mcpServeUsage)
		}
	}
	if check && addr != "" {
		return fmt.Errorf("--check starts no server; drop --http")
	}

	// A gateway chooses the working directory, so only -p selects project mode.
	b := &skillserve.Builder{Target: target}
	var targets map[string]config.TargetConfig
	if mode == modeProject {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		rt, err := loadProjectRuntime(cwd)
		if err != nil {
			return err
		}
		b.Source, b.Walk, targets = rt.sourcePath, rt.skillsWalk(), rt.targets
	} else {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		b.Source, b.Walk, targets = cfg.EffectiveSkillsSource(), cfg.SkillsWalk(), cfg.Targets
	}
	if target != "" {
		tc, ok := targets[target]
		if !ok {
			return fmt.Errorf("unknown target %q", target)
		}
		sc := tc.SkillsConfig()
		if !sc.IsEnabled() {
			return fmt.Errorf("target %q has skills disabled", target)
		}
		b.Skills = &sc
	}
	if check {
		return printMCPServeCheck(b)
	}

	token := os.Getenv(mcpServeTokenEnv)
	if addr != "" && token == "" && !isLoopbackAddr(addr) {
		return fmt.Errorf("refusing to serve on %s without authentication: set %s, or bind a loopback address such as 127.0.0.1:8765", addr, mcpServeTokenEnv)
	}

	srv, err := skillserve.NewServer(b, skillserve.Options{Version: version, Warn: os.Stderr, Refresh: 5 * time.Second})
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if addr == "" {
		return srv.Run(ctx, &mcp.StdioTransport{})
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	httpSrv := &http.Server{Handler: skillserve.HTTPHandler(srv, token), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		httpSrv.Close()
	}()
	fmt.Fprintf(os.Stderr, "Serving skills over MCP at http://%s/\n", ln.Addr())
	if err := httpSrv.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// printMCPServeCheck builds the catalog the server would serve and lists the
// skills it would skip, without starting a server.
func printMCPServeCheck(b *skillserve.Builder) error {
	start := time.Now()
	c, err := b.Build()
	if err != nil {
		return err
	}
	labels := make([]string, len(c.Skipped))
	for i, s := range c.Skipped {
		labels[i] = s.Path
	}
	width := ui.RowWidth(labels...)
	for _, s := range c.Skipped {
		ui.Row(ui.MarkWarn, s.Path, s.Reason, width)
	}
	text, mark := plural(len(c.Skills), "skill")+" served", ui.MarkOK
	if len(c.Skipped) > 0 {
		text, mark = text+fmt.Sprintf(", %d skipped", len(c.Skipped)), ui.MarkWarn
	}
	ui.Done(mark, text, time.Since(start))
	return nil
}

// isLoopbackAddr reports whether a listen address only accepts local
// connections. An empty host listens on every interface.
func isLoopbackAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil || host == "" {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
