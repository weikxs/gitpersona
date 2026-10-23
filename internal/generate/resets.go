package generate

import (
	"cmp"
	"path"
	"strings"

	"github.com/weikxs/gitpersona/internal/config"
	"github.com/weikxs/gitpersona/internal/paths"
)

// KeyRule is one of the user's own includeIf rules that sets signing or an
// SSH command. Dir is the absolute directory it covers, or "" when it can
// apply anywhere (onbranch, hasconfig, the whole home).
type KeyRule struct {
	Dir string
	// Condition is the rule's own includeIf condition, under which the
	// default's reset goes, so it applies exactly where the rule does.
	Condition    string
	Signing, SSH bool
	// Identity is set when the rule sets an identity too: a default profile
	// overrides such a rule, keys included. A rule that only sets keys keeps
	// them under the default, as it did under the user's global identity.
	Identity bool
}

// Resets: a profile that does not sign, or has no SSH key, inherits those
// settings from whatever Git read before it in a repository: the default
// profile (read first, everywhere), a profile whose directory contains the
// repository, or one of the user's own rules. Where that can happen, the
// profile includes a small reset file that puts back the user's own
// settings; elsewhere it leaves them alone, so an SSH program from GIT_SSH
// keeps working there.

// ResetFile returns the reset file of a profile for signing ("signing") or
// the SSH command ("ssh").
func ResetFile(id, what string) string {
	return path.Join(profilesDir, id+".reset-"+what+".gitconfig")
}

// need says which resets a profile needs in one place.
type need struct{ signing, ssh bool }

func (n need) any() bool { return n.signing || n.ssh }

// needAt returns the resets profile p needs at place (a directory, or a
// pinned repository when pin is set).
func (o Options) needAt(cfg config.Config, p config.Profile, place string, pin bool) need {
	var n need
	ownSigning := p.Signing.Method != ""
	ownSSH := p.SSHKey != ""
	for _, other := range cfg.Profiles {
		if other.ID == p.ID || !o.profileReaches(cfg, other, place, pin) {
			continue
		}
		n.signing = n.signing || other.Signing.Method != "" // "none" sets an empty key and signing off
		n.ssh = n.ssh || other.SSHKey != ""
	}
	for _, r := range o.KeyRules {
		if r.Dir == "" || o.overlaps(r.Dir, place) {
			n.signing = n.signing || r.Signing
			n.ssh = n.ssh || r.SSH
		}
	}
	n.signing = n.signing && !ownSigning
	n.ssh = n.ssh && !ownSSH
	return n
}

// profileReaches reports whether other's settings are read before a rule
// for place: other is the default, or one of its directories contains place.
func (o Options) profileReaches(cfg config.Config, other config.Profile, place string, pin bool) bool {
	if other.ID == cfg.Default {
		return true
	}
	for _, d := range other.Dirs {
		if o.contains(d, place, pin) {
			return true
		}
	}
	return false
}

// contains reports whether dir contains place in any form Git matches (see
// pathForms); a pinned repository that is dir itself counts.
func (o Options) contains(dir, place string, pin bool) bool {
	outer, err1 := paths.Normalize(dir, o.Home)
	inner, err2 := paths.Normalize(place, o.Home)
	if err1 != nil || err2 != nil {
		return false
	}
	for _, of := range o.pathForms(outer) {
		for _, in := range o.pathForms(inner) {
			ok, ik := o.compareKey(of), o.compareKey(in)
			if strings.HasPrefix(ik, strings.TrimSuffix(ok, "/")+"/") || (pin && ik == ok) {
				return true
			}
		}
	}
	return false
}

// overlaps reports whether a rule of the user's for dir can apply where a
// profile's rule for place does: around it, inside it, or the same.
func (o Options) overlaps(dir, place string) bool {
	return o.contains(dir, place, true) || o.contains(place, dir, false)
}

// resetPaths returns the reset files to include with profile p's entry for
// place.
func (o Options) resetPaths(cfg config.Config, p config.Profile, place string, pin bool) []string {
	n := o.needAt(cfg, p, place, pin)
	var out []string
	if n.signing {
		out = append(out, path.Join(o.ConfigDir, ResetFile(p.ID, "signing")))
	}
	if n.ssh {
		out = append(out, path.Join(o.ConfigDir, ResetFile(p.ID, "ssh")))
	}
	return out
}

// resetFiles returns the reset files profile p needs anywhere.
func (o Options) resetFiles(cfg config.Config, p config.Profile) []File {
	var n need
	add := func(m need) { n.signing, n.ssh = n.signing || m.signing, n.ssh || m.ssh }
	for _, d := range p.Dirs {
		add(o.needAt(cfg, p, d, false))
	}
	for _, pin := range cfg.Pins {
		if pin.Profile == p.ID {
			add(o.needAt(cfg, p, pin.Repo, true))
		}
	}
	if p.ID == cfg.Default {
		for _, r := range o.KeyRules {
			if r.Identity {
				add(need{signing: r.Signing && p.Signing.Method == "", ssh: r.SSH && p.SSHKey == ""})
			}
		}
	}
	var files []File
	if n.signing {
		var b strings.Builder
		b.WriteString(header)
		writeFallbackSigning(&b, o.FallbackSigning, p.Email)
		files = append(files, File{Name: ResetFile(p.ID, "signing"), Content: b.String()})
	}
	if n.ssh {
		var b strings.Builder
		b.WriteString(header)
		b.WriteString("[core]\n")
		writeKV(&b, "sshCommand", cmp.Or(o.FallbackSSHCommand, "ssh"))
		files = append(files, File{Name: ResetFile(p.ID, "ssh"), Content: b.String()})
	}
	return files
}

// writeDefaultResets writes, after the default profile's include, the
// resets it needs where the user's own rules set signing or an SSH command.
func (o Options) writeDefaultResets(b *strings.Builder, gitdir string, def config.Profile) {
	for _, r := range o.KeyRules {
		n := need{signing: r.Signing && def.Signing.Method == "", ssh: r.SSH && def.SSHKey == ""}
		if !n.any() || !r.Identity {
			continue
		}
		var files []string
		if n.signing {
			files = append(files, path.Join(o.ConfigDir, ResetFile(def.ID, "signing")))
		}
		if n.ssh {
			files = append(files, path.Join(o.ConfigDir, ResetFile(def.ID, "ssh")))
		}
		var headers []string
		switch {
		case r.Condition != "":
			headers = []string{"[includeIf " + quoteSubsection(r.Condition) + "]"}
		case r.Dir != "":
			for _, d := range o.pathForms(r.Dir) {
				headers = append(headers, "[includeIf "+quoteSubsection(gitdir+escapeGlob(d)+"/")+"]")
			}
		default:
			headers = []string{"[include]"}
		}
		for _, h := range headers {
			b.WriteString(h + "\n")
			for _, f := range files {
				writeKV(b, "path", f)
			}
		}
	}
}
