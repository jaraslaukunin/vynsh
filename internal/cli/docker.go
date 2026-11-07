package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/jaraslaukunin/vynsh/internal/fsutil"
	"github.com/jaraslaukunin/vynsh/internal/style"
	"github.com/jaraslaukunin/vynsh/internal/units"
)

// dockerRow mirrors `docker system df` for one resource type.
type dockerRow struct {
	Type       string `json:"type"`
	Count      int64  `json:"count"`
	Size       int64  `json:"size"`
	Reclaim    int64  `json:"reclaimable"`
	SizeText   string `json:"size_text"`
	ReclaimTxt string `json:"reclaimable_text"`
}

// dockerInfo is the JSON shape of `vynsh docker`.
type dockerInfo struct {
	Available      bool        `json:"available"`
	Version        string      `json:"version,omitempty"`
	DaemonRunning  bool        `json:"daemon_running"`
	Rows           []dockerRow `json:"rows,omitempty"`
	Reclaimable    int64       `json:"reclaimable"`
	DataImage      string      `json:"data_image,omitempty"`
	DataImageBytes int64       `json:"data_image_bytes,omitempty"`
	DataImageAlloc int64       `json:"data_image_allocated,omitempty"`
	Error          string      `json:"error,omitempty"`
}

func newDockerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "docker",
		Aliases: []string{"orb", "orbstack"},
		Short:   "Show and reclaim space used by Docker / OrbStack",
		Long: `Docker keeps images, containers, volumes and build cache — often tens of
gigabytes that a normal file scan cannot attribute, because they live inside the
VM disk image.

This command reads "docker system df", shows the container disk image next to it
and can run "docker system prune" for you.

Examples:
  vynsh docker                  # what Docker uses and what is reclaimable
  vynsh docker prune            # remove unused containers, networks, images
  vynsh docker prune --all      # also remove images not used by any container
  vynsh docker prune --yes      # unattended (scripts, CI)`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			info := collectDocker()
			if g.json {
				return mustJSON(info)
			}
			printDocker(cmd.OutOrStdout(), info)
			return nil
		},
	}
	cmd.AddCommand(newDockerPruneCmd())
	return cmd
}

func newDockerPruneCmd() *cobra.Command {
	var (
		all     bool
		volumes bool
		dryRun  bool
	)
	cmd := &cobra.Command{
		Use:   "prune",
		Short: "Remove unused Docker data",
		Long: `Run "docker system prune" and report how much space was freed.

Nothing is pruned without confirmation; --all also removes images not referenced
by any container, --volumes removes unused volumes (this can delete data).`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			path, err := exec.LookPath("docker")
			if err != nil {
				return fmt.Errorf("docker CLI not found in PATH — install Docker Desktop, OrbStack or the docker engine")
			}
			info := collectDocker()
			if !info.DaemonRunning {
				return fmt.Errorf("the Docker daemon is not running — start Docker Desktop / OrbStack and try again")
			}
			if !g.json {
				printDocker(out, info)
			}
			pruneArgs := []string{"system", "prune", "-f"}
			if all {
				pruneArgs = append(pruneArgs, "-a")
			}
			if volumes {
				pruneArgs = append(pruneArgs, "--volumes")
			}
			what := "docker " + strings.Join(pruneArgs, " ")
			if dryRun || !mayDelete(out, fmt.Sprintf("Run %q and remove %s of unused data?",
				what, units.HumanBytes(info.Reclaimable)),
				"Dry run: nothing was pruned. Re-run in a terminal to confirm, or pass --yes.") {
				if dryRun {
					fmt.Fprintf(out, "\n%s\n", g.th.Dim("Would run: "+what))
				}
				return nil
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			before := info.Reclaimable
			proc := exec.CommandContext(ctx, path, pruneArgs...)
			output, err := proc.CombinedOutput()
			if err != nil {
				return fmt.Errorf("docker prune failed: %w\n%s", err, strings.TrimSpace(string(output)))
			}
			after := collectDocker()
			if g.json {
				return mustJSON(map[string]any{
					"command":  what,
					"output":   strings.TrimSpace(string(output)),
					"before":   before,
					"reported": strings.TrimSpace(string(output)),
					"after":    after,
				})
			}
			printCommandOutput(out, string(output))
			fmt.Fprintf(out, "%s pruned %s\n", g.th.Green("✓"),
				g.th.Bold(units.HumanBytes(maxInt64(0, before-dockerUsed(after)))))
			return nil
		},
	}
	cmd.Flags().BoolVarP(&all, "all", "a", false, "also remove images not used by any container")
	cmd.Flags().BoolVar(&volumes, "volumes", false, "also remove unused volumes (data loss possible)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show the command, prune nothing")
	return cmd
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

// dockerUsed sums the current size of all Docker resource types.
func dockerUsed(info dockerInfo) int64 {
	var sum int64
	for _, r := range info.Rows {
		sum += r.Size
	}
	return sum
}

// collectDocker gathers everything the docker command shows.
func collectDocker() dockerInfo {
	info := dockerInfo{}
	path, err := exec.LookPath("docker")
	if err != nil {
		info.Error = "docker CLI not found in PATH"
		info.DataImage, info.DataImageBytes = dockerDataImage()
		return info
	}
	info.Available = true
	if v, err := dockerOutput(path, 8*time.Second, "version", "--format", "{{.Server.Version}}"); err == nil {
		info.Version = strings.TrimSpace(v)
		info.DaemonRunning = info.Version != ""
	} else if g.verbose {
		fmt.Fprintf(os.Stderr, "vynsh: docker version: %v\n", err)
	}
	if info.DaemonRunning {
		if rows, err := dockerDF(path); err == nil {
			info.Rows = rows
			for _, r := range rows {
				info.Reclaimable += r.Reclaim
			}
		} else if info.Error == "" {
			info.Error = err.Error()
		}
	}
	info.DataImage, info.DataImageBytes = dockerDataImage()
	if info.DataImage != "" {
		info.DataImageAlloc = fsutil.Allocated(info.DataImage)
	}
	return info
}

func dockerOutput(bin string, timeout time.Duration, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("docker %s timed out", strings.Join(args, " "))
		}
		return "", err
	}
	return string(out), nil
}

// dockerDF parses `docker system df` into structured rows.
func dockerDF(bin string) ([]dockerRow, error) {
	out, err := dockerOutput(bin, 20*time.Second, "system", "df", "--format",
		"{{.Type}}\t{{.TotalCount}}\t{{.Size}}\t{{.Reclaimable}}")
	if err != nil {
		return nil, fmt.Errorf("docker system df failed: %w", err)
	}
	var rows []dockerRow
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 4 {
			continue
		}
		row := dockerRow{
			Type:       fields[0],
			SizeText:   strings.TrimSpace(fields[2]),
			ReclaimTxt: strings.TrimSpace(fields[3]),
		}
		row.Count, _ = parseInt64(strings.TrimSpace(fields[1]))
		row.Size = parseDockerSize(row.SizeText)
		row.Reclaim = parseDockerSize(row.ReclaimTxt)
		rows = append(rows, row)
	}
	return rows, sc.Err()
}

func parseInt64(s string) (int64, error) {
	var v int64
	_, err := fmt.Sscanf(s, "%d", &v)
	return v, err
}

// parseDockerSize turns "1.234GB (50%)" or "0B" into bytes.
func parseDockerSize(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" || s == "N/A" {
		return 0
	}
	if i := strings.IndexAny(s, " ("); i > 0 {
		s = s[:i]
	}
	s = strings.TrimSpace(s)
	// Docker uses kB/MB/GB (base 1000) but older versions used B-only values.
	s = strings.ReplaceAll(s, "kB", "KB")
	v, err := units.ParseBytes(s)
	if err != nil {
		return 0
	}
	return v
}

// dockerDataImage locates the VM disk image that holds everything Docker knows.
func dockerDataImage() (string, int64) {
	candidates := []string{}
	if runtime.GOOS == "windows" {
		base := os.Getenv("LOCALAPPDATA")
		candidates = append(candidates,
			filepath.Join(base, "Docker", "wsl", "data", "ext4.vhdx"),
			filepath.Join(base, "Docker", "wsl", "disk", "docker_data.vhdx"),
			filepath.Join(base, "Docker", "wsl", "disk", "docker_desktop.vhdx"),
		)
	} else {
		home := fsutil.HomeDir()
		candidates = append(candidates,
			filepath.Join(home, "Library", "Containers", "com.docker.docker", "Data", "vms", "0", "data", "Docker.raw"),
			filepath.Join(home, "Library", "Containers", "com.docker.docker", "Data", "vms", "0", "data", "Docker.qcow2"),
			filepath.Join(home, ".orbstack", "data", "data.img"),
			filepath.Join(home, ".colima", "default", "colima", "diffdisk"),
			"/var/lib/docker",
		)
	}
	for _, p := range candidates {
		st, err := os.Stat(p)
		if err != nil {
			continue
		}
		if st.IsDir() {
			return p, fsutil.DirSize(p).Bytes
		}
		return p, st.Size()
	}
	return "", 0
}

func printDocker(w io.Writer, info dockerInfo) {
	th := g.th
	if !info.Available {
		fmt.Fprintf(w, "%s %s\n", th.Yellow("Docker:"), info.Error)
		fmt.Fprintf(w, "%s\n", th.Dim("Install Docker Desktop, OrbStack or the docker engine to use this command."))
		if info.DataImage != "" {
			fmt.Fprintf(w, "%s %s\n", th.Dim("disk image:"), fmt.Sprintf("%s — %s", shortenPath(info.DataImage), units.HumanBytes(info.DataImageBytes)))
		}
		return
	}
	status := th.Green("running")
	if !info.DaemonRunning {
		status = th.Yellow("daemon not running")
	}
	version := "docker"
	if info.Version != "" {
		version = "v" + info.Version
	}
	fmt.Fprintf(w, "%s %s · %s\n", th.Dim("docker:"), th.Bold(version), status)
	if info.DaemonRunning {
		rows := make([][]string, 0, len(info.Rows))
		for _, r := range info.Rows {
			rows = append(rows, []string{
				r.Type,
				units.HumanCount(r.Count),
				r.SizeText,
				th.Yellow(r.ReclaimTxt),
			})
		}
		t := &style.Table{
			Headers:  []string{"TYPE", "COUNT", "SIZE", "RECLAIMABLE"},
			Rows:     rows,
			Align:    []style.Align{style.Left, style.Right, style.Right, style.Right},
			MaxWidth: []int{20, 8, 12, 16},
		}
		fmt.Fprint(w, t.Render(th))
		fmt.Fprintf(w, "\n%s %s\n", th.Dim("total reclaimable:"), th.Bold(units.HumanBytes(info.Reclaimable)))
	} else {
		fmt.Fprintf(w, "%s\n", th.Dim("Start Docker Desktop / OrbStack (or `colima start`) to see images, containers and build cache."))
	}
	if info.DataImage != "" {
		fmt.Fprintf(w, "%s %s — %s on disk", th.Dim("disk image:"),
			shortenPath(info.DataImage), units.HumanBytes(info.DataImageAlloc))
		if info.DataImageBytes > info.DataImageAlloc*3/2 {
			fmt.Fprintf(w, " %s", th.Dim(fmt.Sprintf("(%s apparent, sparse)", units.HumanBytes(info.DataImageBytes))))
		}
	}
	if info.DaemonRunning && info.Reclaimable > 0 {
		fmt.Fprintf(w, "\n%s\n", th.Dim("reclaim it: vynsh docker prune"))
	}
}

// printCommandOutput prints docker's own output, indented and dimmed.
func printCommandOutput(w io.Writer, output string) {
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		fmt.Fprintf(w, "  %s\n", g.th.Dim(line))
	}
}
