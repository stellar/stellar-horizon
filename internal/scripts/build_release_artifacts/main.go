package main

// This is a build script that Travis uses to build Stellar release packages.

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/stellar/go-stellar-sdk/support/errors"
	"github.com/stellar/go-stellar-sdk/support/log"
)

var builds = []buildConfig{
	{"darwin", "amd64"},
	{"linux", "amd64"},
	{"linux", "arm"},
	{"windows", "amd64"},
}

var osFilter = flag.String("os", "", "restrict build to single os")
var archFilter = flag.String("arch", "", "restrict build to single arch")
var keepDir = flag.Bool("keep", false, "when true, artifact directories are not removed after packaging")

type buildConfig struct {
	OS   string
	Arch string
}

func main() {
	flag.Parse()
	log.SetLevel(log.InfoLevel)
	run("rm", "-rf", "dist/*")

	if ghTag := getGitHubTagName(); ghTag != "" {
		buildByTag(ghTag)
		os.Exit(0)
	} else {
		buildSnapshots()
		os.Exit(0)
	}

	log.Info("nothing to do")
}

func getGitHubTagName() string {
	const githubTagRefPrefix = "refs/tags/"
	ref := os.Getenv("GITHUB_REF")
	if ref == "" {
		// we are not in a github action
		return ""
	}
	if !strings.HasPrefix(ref, githubTagRefPrefix) {
		// we are not in a tag-triggered event
		return ""
	}
	return strings.TrimPrefix(ref, githubTagRefPrefix)
}

func build(pkg, dest, version, buildOS, buildArch string) {
	// Note: verison string should match other build pipelines to create
	// reproducible builds for Horizon (and other projects in the future).
	rev := runOutput("git", "rev-parse", "HEAD")
	versionString := version[1:] // Remove letter `v`
	versionFlag := fmt.Sprintf(
		"-X=github.com/stellar/go-stellar-sdk/support/app.version=%s-%s",
		versionString, rev,
	)

	if buildOS == "windows" {
		dest = dest + ".exe"
	}

	cmd := exec.Command("go", "build",
		"-trimpath",
		"-o", dest,
		pkg,
	)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout

	cmd.Env = append(
		os.Environ(),
		"CGO_ENABLED=0",
		fmt.Sprintf("GOFLAGS=-ldflags=%s", versionFlag),
		fmt.Sprintf("GOOS=%s", buildOS),
		fmt.Sprintf("GOARCH=%s", buildArch),
	)
	log.Infof("building %s", pkg)

	log.Infof("running: %s", strings.Join(cmd.Args, " "))
	err := cmd.Run()
	if err != nil {
		panic(err)
	}
}

func buildByTag(tag string) {
	repo := repoName()

	for _, cfg := range getBuildConfigs() {
		dest := prepareDest(".", "horizon", tag, cfg.OS, cfg.Arch)

		// rebuild the binary with the version variable set
		build(
			fmt.Sprintf("%s/.", repo),
			filepath.Join(dest, "horizon"),
			tag,
			cfg.OS,
			cfg.Arch,
		)

		packageArchive(dest, cfg.OS)
	}
}

func buildSnapshots() {
	rev := runOutput("git", "describe", "--always", "--dirty")
	version := fmt.Sprintf("snapshot-%s", rev)
	repo := repoName()

	for _, cfg := range getBuildConfigs() {
		dest := prepareDest(".", "horizon", "snapshot", cfg.OS, cfg.Arch)

		build(
			fmt.Sprintf("%s/.", repo),
			filepath.Join(dest, "horizon"),
			version,
			cfg.OS,
			cfg.Arch,
		)

		packageArchive(dest, cfg.OS)
	}
}

func getBuildConfigs() (result []buildConfig) {
	for _, cfg := range builds {

		if *osFilter != "" && *osFilter != cfg.OS {
			continue
		}

		if *archFilter != "" && *archFilter != cfg.Arch {
			continue
		}

		result = append(result, cfg)
	}
	return
}

// packageArchive tars or zips `dest`, depending upon the OS, then removes
// `dest`, in preparation of Circle uploading all artifacts to github releases.
func packageArchive(dest, buildOS string) {
	release := filepath.Base(dest)
	dir := filepath.Dir(dest)

	if buildOS == "windows" {
		pop := pushdir(dir)
		// zip $RELEASE.zip $RELEASE/*
		run("zip", "-r", release+".zip", release)
		pop()
	} else {
		// tar -czf $dest.tar.gz -C $DIST $RELEASE
		run("tar", "-czf", dest+".tar.gz", "-C", dir, release)
	}

	if !*keepDir {
		run("rm", "-rf", dest)
	}
}

func prepareDest(pkg, bin, version, os, arch string) string {
	name := fmt.Sprintf("%s-%s-%s-%s", bin, version, os, arch)
	dest := filepath.Join("dist", name)

	// make destination directories
	run("mkdir", "-p", dest)
	run("cp", "LICENSE", dest)
	run("cp", "COPYING", dest)
	run("cp", filepath.Join(pkg, "README.md"), dest)
	run("cp", filepath.Join(pkg, "CHANGELOG.md"), dest)
	return dest
}

// pushdir is a utility function to temporarily change directories.  It returns
// a func that can be called to restore the current working directory to the
// state it was in when first calling pushdir.
func pushdir(dir string) func() {
	cwd, err := os.Getwd()
	if err != nil {
		panic(errors.Wrap(err, "getwd failed"))
	}

	err = os.Chdir(dir)
	if err != nil {
		panic(errors.Wrap(err, "chdir failed"))
	}

	return func() {
		err := os.Chdir(cwd)
		if err != nil {
			panic(errors.Wrap(err, "revert dir failed"))
		}
	}
}

func repoName() string {
	if os.Getenv("REPO") != "" {
		return os.Getenv("REPO")
	}
	return "github.com/stellar/stellar-horizon"

}

// utility command to run the provided command that echoes any output.  A failed
// command will trigger a panic.
func run(name string, args ...string) {
	cmd := exec.Command(name, args...)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout

	log.Infof("running: %s %s", name, strings.Join(args, " "))
	err := cmd.Run()

	if err != nil {
		panic(err)
	}
}

// utility command to run  the provided command that returns the output.  A
// failed command will trigger a panic.
func runOutput(name string, args ...string) string {
	cmd := exec.Command(name, args...)
	cmd.Stderr = os.Stderr

	log.Infof("running: %s %s", name, strings.Join(args, " "))
	out, err := cmd.Output()

	if err != nil {
		panic(err)
	}

	return strings.TrimSpace(string(out))
}
