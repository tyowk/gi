package gipack

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Manifest struct {
	Name            string            `json:"name"`
	Version         string            `json:"version"`
	Description     string            `json:"description,omitempty"`
	Author          string            `json:"author,omitempty"`
	License         string            `json:"license,omitempty"`
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies,omitempty"`
	Scripts         map[string]string `json:"scripts,omitempty"`
}

type LockEntry struct {
	URL     string `json:"url"`
	Commit  string `json:"commit"`
	Pinned  bool   `json:"pinned,omitempty"`
	Updated string `json:"updated"`
}

type Lockfile struct {
	Packages map[string]LockEntry `json:"packages"`
}

const (
	ManifestFile = "gipack.json"
	LockfileName = "gipack.lock"
	ModulesDir   = "gipack_modules"
)

func Init() error {
	if _, err := os.Stat(ManifestFile); err == nil {
		return fmt.Errorf("%s already exists in this directory", ManifestFile)
	}

	cwd, _ := os.Getwd()
	name := filepath.Base(cwd)

	m := Manifest{
		Name:            name,
		Version:         "0.1.0",
		Description:     "",
		Dependencies:    map[string]string{},
		DevDependencies: map[string]string{},
		Scripts: map[string]string{
			"start": "gi run main.gi",
			"check": "gi check main.gi",
		},
	}
	if err := writeManifest(m); err != nil {
		return err
	}
	fmt.Printf("Created %s\n", ManifestFile)
	fmt.Printf("Run 'gi pack install <user/repo>' to add a package.\n")
	return nil
}

func Install(spec string) error {
	if spec == "" {
		return installAll()
	}
	return installOne(spec, true)
}

func Add(spec string, dev bool) error {
	return installOne(spec, true, dev)
}

func Remove(name string) error {
	m, err := readManifest()
	if err != nil {
		return err
	}
	_, okDep := m.Dependencies[name]
	_, okDev := m.DevDependencies[name]
	if !okDep && !okDev {
		return fmt.Errorf("package %q is not listed in %s", name, ManifestFile)
	}

	modPath := filepath.Join(ModulesDir, name)
	if err := os.RemoveAll(modPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("cannot remove %s: %v", modPath, err)
	}

	delete(m.Dependencies, name)
	delete(m.DevDependencies, name)
	if err := writeManifest(m); err != nil {
		return err
	}

	lf := readLockfile()
	delete(lf.Packages, name)
	writeLockfile(lf)

	fmt.Printf("Removed %q\n", name)
	return nil
}

func Update(name string) error {
	m, err := readManifest()
	if err != nil {
		return err
	}
	if name != "" {
		_, okDep := m.Dependencies[name]
		_, okDev := m.DevDependencies[name]
		if !okDep && !okDev {
			return fmt.Errorf("package %q not found in %s", name, ManifestFile)
		}
		return updateOne(name)
	}
	for pkgName := range m.Dependencies {
		if err := updateOne(pkgName); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not update %q: %v\n", pkgName, err)
		}
	}
	for pkgName := range m.DevDependencies {
		if err := updateOne(pkgName); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not update %q: %v\n", pkgName, err)
		}
	}
	return nil
}

func List() error {
	m, err := readManifest()
	if err != nil {
		return err
	}
	lf := readLockfile()

	if len(m.Dependencies) == 0 && len(m.DevDependencies) == 0 {
		fmt.Println("No packages installed. Run 'gi pack add <user/repo>' to add one.")
		return nil
	}

	names := make([]string, 0, len(m.Dependencies)+len(m.DevDependencies))
	for n := range m.Dependencies {
		names = append(names, n)
	}
	for n := range m.DevDependencies {
		if _, exists := m.Dependencies[n]; !exists {
			names = append(names, n)
		}
	}
	sort.Strings(names)

	fmt.Printf("%-20s  %-8s  %-40s  %s\n", "NAME", "TYPE", "SOURCE", "COMMIT")
	fmt.Println(strings.Repeat("-", 80))
	for _, n := range names {
		src := m.Dependencies[n]
		typ := "dep"
		if src == "" {
			src = m.DevDependencies[n]
			typ = "devDep"
		}
		commit := "(not locked)"
		if entry, ok := lf.Packages[n]; ok {
			commit = entry.Commit
			if len(commit) > 12 {
				commit = commit[:12]
			}
		}
		installed := "(missing)"
		if _, err := os.Stat(filepath.Join(ModulesDir, n)); err == nil {
			installed = ""
		}
		if installed != "" {
			commit = installed
		}
		fmt.Printf("%-20s  %-8s  %-40s  %s\n", n, typ, src, commit)
	}
	return nil
}

func Info(name string) error {
	modPath := filepath.Join(ModulesDir, name)
	if _, err := os.Stat(modPath); os.IsNotExist(err) {
		return fmt.Errorf("package %q is not installed (run 'gi pack install %s')", name, name)
	}

	lf := readLockfile()
	entry, hasLock := lf.Packages[name]

	fmt.Printf("Package:  %s\n", name)
	if hasLock {
		fmt.Printf("URL:      %s\n", entry.URL)
		fmt.Printf("Commit:   %s\n", entry.Commit)
		fmt.Printf("Updated:  %s\n", entry.Updated)
	}

	var giFiles []string
	filepath.Walk(modPath, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(p, ".gi") {
			rel, _ := filepath.Rel(modPath, p)
			giFiles = append(giFiles, rel)
		}
		return nil
	})
	fmt.Printf("Files:    %d .gi file(s)\n", len(giFiles))
	for _, f := range giFiles {
		fmt.Printf("          %s\n", f)
	}

	pkgManifest := filepath.Join(modPath, ManifestFile)
	if data, err := os.ReadFile(pkgManifest); err == nil {
		var pm Manifest
		if json.Unmarshal(data, &pm) == nil {
			if pm.Description != "" {
				fmt.Printf("Desc:     %s\n", pm.Description)
			}
			if pm.Author != "" {
				fmt.Printf("Author:   %s\n", pm.Author)
			}
			if pm.License != "" {
				fmt.Printf("License:  %s\n", pm.License)
			}
		}
	}
	return nil
}

func installAll() error {
	m, err := readManifest()
	if err != nil {
		return err
	}
	if len(m.Dependencies) == 0 && len(m.DevDependencies) == 0 {
		fmt.Println("No dependencies listed in gipack.json.")
		return nil
	}
	for name := range m.Dependencies {
		src := m.Dependencies[name]
		if err := cloneOrSkip(name, src, false); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to install %q: %v\n", name, err)
		}
	}
	for name := range m.DevDependencies {
		src := m.DevDependencies[name]
		if err := cloneOrSkip(name, src, false); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to install %q: %v\n", name, err)
		}
	}
	return nil
}

func installOne(spec string, save bool, dev ...bool) error {
	url, name := resolveSpec(spec)
	isDev := len(dev) > 0 && dev[0]

	m, merr := readManifest()
	if merr != nil {
		m = Manifest{
			Name:            filepath.Base(mustCWD()),
			Version:         "0.1.0",
			Dependencies:    map[string]string{},
			DevDependencies: map[string]string{},
			Scripts:         map[string]string{},
		}
	}

	if err := cloneOrSkip(name, url, true); err != nil {
		return err
	}

	if save {
		ensureManifestMaps(&m)
		if isDev {
			delete(m.Dependencies, name)
			m.DevDependencies[name] = spec
		} else {
			delete(m.DevDependencies, name)
			m.Dependencies[name] = spec
		}
		if err := writeManifest(m); err != nil {
			return fmt.Errorf("could not update %s: %v", ManifestFile, err)
		}
	}
	return nil
}

func RunScript(name string, args []string) error {
	m, err := readManifest()
	if err != nil {
		return err
	}
	command, ok := m.Scripts[name]
	if !ok || strings.TrimSpace(command) == "" {
		return fmt.Errorf("script %q not found in %s", name, ManifestFile)
	}
	if len(args) > 0 {
		command += " " + strings.Join(args, " ")
	}
	cmd := exec.Command("bash", "-lc", command)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func cloneOrSkip(name, spec string, verbose bool) error {
	url, _ := resolveSpec(spec)
	modPath := filepath.Join(ModulesDir, name)

	if _, err := os.Stat(modPath); err == nil {
		if verbose {
			fmt.Printf("  → %s already installed (use 'gi pack update %s' to upgrade)\n", name, name)
		}
		return nil
	}

	if err := os.MkdirAll(ModulesDir, 0755); err != nil {
		return err
	}

	if !gitAvailable() {
		return fmt.Errorf("git is not installed or not on PATH — needed to fetch packages")
	}

	fmt.Printf("  Installing %s from %s ...\n", name, url)
	cmd := exec.Command("git", "clone", "--depth=1", "--quiet", url, modPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git clone failed: %v", err)
	}

	commit := gitCommit(modPath)
	lf := readLockfile()
	lf.Packages[name] = LockEntry{
		URL:     url,
		Commit:  commit,
		Updated: time.Now().Format(time.RFC3339),
	}
	writeLockfile(lf)

	fmt.Printf("  ✓ Installed %s @ %s\n", name, shortHash(commit))
	return nil
}

func updateOne(name string) error {
	m, err := readManifest()
	if err != nil {
		return err
	}

	spec, ok := m.Dependencies[name]
	if !ok {
		spec, ok = m.DevDependencies[name]
		if !ok {
			return fmt.Errorf("package %q not in manifest", name)
		}
	}

	url, _ := resolveSpec(spec)
	modPath := filepath.Join(ModulesDir, name)
	if _, err := os.Stat(modPath); os.IsNotExist(err) {
		return cloneOrSkip(name, url, true)
	}

	if !gitAvailable() {
		return fmt.Errorf("git not available")
	}

	fmt.Printf("  Updating %s ...\n", name)
	cmd := exec.Command("git", "-C", modPath, "pull", "--quiet", "--depth=1")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git pull failed: %v", err)
	}

	commit := gitCommit(modPath)
	lf := readLockfile()
	lf.Packages[name] = LockEntry{
		URL:     url,
		Commit:  commit,
		Updated: time.Now().Format(time.RFC3339),
	}
	writeLockfile(lf)

	fmt.Printf("  ✓ Updated %s @ %s\n", name, shortHash(commit))
	return nil
}

func resolveSpec(spec string) (url, name string) {
	spec = strings.TrimSuffix(spec, ".git")
	parts := strings.Split(spec, "/")

	switch {
	case strings.HasPrefix(spec, "https://") || strings.HasPrefix(spec, "http://"):
		name = parts[len(parts)-1]
		url = spec + ".git"
	case len(parts) == 3 && (parts[0] == "github.com" || parts[0] == "gitlab.com" || parts[0] == "bitbucket.org"):
		url = "https://" + spec + ".git"
		name = parts[2]
	case len(parts) == 2:
		url = "https://github.com/" + spec + ".git"
		name = parts[1]
	default:
		url = spec
		name = spec
	}
	return
}

func gitCommit(dir string) string {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

func shortHash(h string) string {
	if len(h) > 12 {
		return h[:12]
	}
	return h
}

func gitAvailable() bool {
	_, err := exec.LookPath("git")
	return err == nil
}

func ResolveImport(importPath, baseDir string) (string, error) {
	if strings.HasPrefix(importPath, "/") {
		return ensureGi(importPath), nil
	}

	if strings.HasPrefix(importPath, "./") || strings.HasPrefix(importPath, "../") {
		p := filepath.Join(baseDir, importPath)
		return ensureGi(p), nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		cwd = baseDir
	}

	candidates := []string{
		filepath.Join(cwd, ModulesDir, importPath+".gi"),
		filepath.Join(cwd, ModulesDir, importPath, "main.gi"),
		filepath.Join(cwd, ModulesDir, importPath, "init.gi"),
		filepath.Join(baseDir, ModulesDir, importPath+".gi"),
		filepath.Join(baseDir, ModulesDir, importPath, "main.gi"),
	}

	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}

	p := filepath.Join(baseDir, importPath)
	return ensureGi(p), nil
}

func ensureGi(p string) string {
	if !strings.HasSuffix(p, ".gi") {
		return p + ".gi"
	}
	return p
}

func readManifest() (Manifest, error) {
	data, err := os.ReadFile(ManifestFile)
	if err != nil {
		return Manifest{}, fmt.Errorf(
			"%s not found — run 'gi pack init' to create one", ManifestFile)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return Manifest{}, fmt.Errorf("malformed %s: %v", ManifestFile, err)
	}
	ensureManifestMaps(&m)
	return m, nil
}

func writeManifest(m Manifest) error {
	ensureManifestMaps(&m)
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(ManifestFile, append(data, '\n'), 0644)
}

func readLockfile() Lockfile {
	data, err := os.ReadFile(LockfileName)
	if err != nil {
		return Lockfile{Packages: map[string]LockEntry{}}
	}
	var lf Lockfile
	if err := json.Unmarshal(data, &lf); err != nil {
		return Lockfile{Packages: map[string]LockEntry{}}
	}
	if lf.Packages == nil {
		lf.Packages = map[string]LockEntry{}
	}
	return lf
}

func writeLockfile(lf Lockfile) {
	data, _ := json.MarshalIndent(lf, "", "  ")
	os.WriteFile(LockfileName, append(data, '\n'), 0644)
}

func mustCWD() string {
	cwd, _ := os.Getwd()
	return cwd
}

func ensureManifestMaps(m *Manifest) {
	if m.Dependencies == nil {
		m.Dependencies = map[string]string{}
	}
	if m.DevDependencies == nil {
		m.DevDependencies = map[string]string{}
	}
	if m.Scripts == nil {
		m.Scripts = map[string]string{}
	}
}
