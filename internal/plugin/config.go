package plugin

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
	"skillshare/internal/utils"
)

type document struct {
	raw      []byte
	node     yaml.Node
	packages map[string]Package
}

func (s *Service) load() (*document, error) {
	raw, err := os.ReadFile(s.ConfigPath)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return decodeDocument(raw, s.accountAgents())
}

// Validate checks plugin declarations without reading or writing native state. accounts
// maps each target that is another config directory of an Agent to that Agent, so a
// binding is validated by the Agent that would install it.
func Validate(raw []byte, accounts map[string]string) error {
	_, err := decodeDocument(raw, accounts)
	return err
}

// decodeDocument reads the plugin section. accounts maps each account to its Agent, or to
// "" where only the names are known.
func decodeDocument(raw []byte, accounts map[string]string) (*document, error) {
	d := &document{raw: raw, packages: map[string]Package{}}
	content := raw
	if len(content) == 0 {
		content = []byte("{}\n")
	}
	decoder := yaml.NewDecoder(bytes.NewReader(content))
	if err := decoder.Decode(&d.node); err != nil {
		return nil, err
	}
	if decoder.Decode(new(yaml.Node)) != io.EOF {
		return nil, fmt.Errorf("configuration must contain one YAML document")
	}
	var check map[string]any
	if err := d.node.Decode(&check); err != nil {
		return nil, err
	}
	if len(d.node.Content) != 1 || d.node.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("configuration must be a YAML mapping")
	}
	root := d.node.Content[0]
	for i := 0; i < len(root.Content); i += 2 {
		if root.Content[i].Value == "plugins" {
			var body struct {
				Packages map[string]Package `yaml:"packages"`
			}
			data, err := yaml.Marshal(root.Content[i+1])
			if err != nil {
				return nil, err
			}
			dec := yaml.NewDecoder(bytes.NewReader(data))
			dec.KnownFields(true)
			if err := dec.Decode(&body); err != nil {
				return nil, fmt.Errorf("invalid plugins configuration: %w", err)
			}
			if body.Packages != nil {
				d.packages = body.Packages
			}
		}
	}
	for name, p := range d.packages {
		if !namePattern.MatchString(name) {
			return nil, fmt.Errorf("invalid plugin package name %q", name)
		}
		p.Source = canonicalSource(p.Source)
		for target, b := range p.Bindings {
			b.Source = canonicalSource(b.Source)
			p.Bindings[target] = b
			agent, account := accounts[target]
			if !account && !slices.Contains(Targets, target) {
				return nil, fmt.Errorf("invalid plugin binding for %s", name)
			}
			if !account {
				agent = target
			}
			if b.PiRegistration != "" && (agent != "pi" || !piRegistrationDigestOK(b.PiRegistration)) {
				return nil, fmt.Errorf("invalid preserved Pi registration for %s", name)
			}
			if !validTargetID(agent, b.ID) {
				return nil, fmt.Errorf("invalid plugin binding for %s", name)
			}
		}
		d.packages[name] = p
	}
	return d, nil
}

// canonicalSource spells a local source the way add records it, so ~/plug and
// /home/me/plug name the same snapshot and owner.
func canonicalSource(source string) string {
	if strings.HasPrefix(source, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, source[2:])
		}
	}
	if filepath.IsAbs(source) {
		return filepath.Clean(source)
	}
	return source
}

// foldSources returns packages with local sources under home written as ~/...
func foldSources(packages map[string]Package) map[string]Package {
	home, err := os.UserHomeDir()
	if err != nil {
		return packages
	}
	out := make(map[string]Package, len(packages))
	for name, p := range packages {
		p.Source = utils.FoldHomePathWith(p.Source, home)
		bindings := make(map[string]Binding, len(p.Bindings))
		for target, b := range p.Bindings {
			b.Source = utils.FoldHomePathWith(b.Source, home)
			bindings[target] = b
		}
		p.Bindings = bindings
		out[name] = p
	}
	return out
}

func (s *Service) save(d *document) error {
	current, err := os.ReadFile(s.ConfigPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if !bytes.Equal(current, d.raw) {
		return fmt.Errorf("configuration changed; preview again")
	}
	packages := d.packages
	var opts struct {
		PreserveTilde bool `yaml:"preserve_tilde_on_save"`
	}
	if d.node.Decode(&opts) == nil && opts.PreserveTilde {
		packages = foldSources(packages)
	}
	var n yaml.Node
	if err := n.Encode(struct {
		Packages map[string]Package `yaml:"packages"`
	}{packages}); err != nil {
		return err
	}
	root := d.node.Content[0]
	found := false
	for i := 0; i < len(root.Content); i += 2 {
		if root.Content[i].Value == "plugins" {
			root.Content[i+1] = &n
			found = true
		}
	}
	if !found {
		root.Content = append(root.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "plugins"}, &n)
	}
	data, err := utils.MarshalYAML(&d.node)
	if err != nil {
		return err
	}
	// Dotfile managers often symlink config.yaml; write its target so the link survives.
	path := utils.ResolveSymlink(s.ConfigPath)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".plugins-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	mode := os.FileMode(0600)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	if err = f.Chmod(mode); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = os.Rename(f.Name(), path); err == nil {
		d.raw = data
	}
	return err
}
