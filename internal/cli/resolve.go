package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
)

// Resolved is the output of walking argv against a verb tree.
//
// Leaf is the resolved Command. It may itself be a *Group when the user typed
// only a group prefix (e.g. `ana profile`) — Dispatch renders the group's
// help in that case.
//
// Path lists the *Group ancestors descended through, root first. The leaf's
// parent (if any) is the last entry; Path always contains at least root.
//
// MergedFS holds every persistent flag declared by any ancestor on Path plus
// every local flag the leaf declared via Flagger. Names a leaf re-declares
// SHADOW the ancestor of the same name (leaf-wins) — see Resolve below for
// the mechanism. The FlagSet has already been parsed against argv's flag
// tokens by the time Resolve returns; leaves call FlagSetFrom(ctx) when they
// need post-parse helpers like RequireFlags / FlagWasSet.
//
// Args is the leaf's positional remainder after parsing.
type Resolved struct {
	Leaf     Command
	Path     []*Group
	MergedFS *flag.FlagSet
	Args     []string
}

// Resolve walks args left-to-right against root, descending into Children
// when it encounters a positional matching a child name. Flag tokens are
// SKIPPED during the walk; we use a shape-only FlagSet built from
// already-traversed ancestor declarations to know which names take a value
// (so a `--profile prod chat` argv doesn't mistake `prod` for the verb).
//
// Once the leaf is identified, Resolve assembles MergedFS in leaf-wins order:
// the leaf's own Flagger.Flags is called first; then each ancestor Group's
// Flags closure is invoked on a private shadow FlagSet, and only the entries
// whose names are NOT already on MergedFS are copied across. That way an
// ancestor declaration of, say, `--endpoint` is silently superseded by a leaf
// declaration of the same name — making the "global flag stomps leaf flag"
// class of bug structurally impossible.
//
// Errors:
//   - unknown verb name during walk → usage-wrapped error. Resolved is
//     non-nil with Leaf set to the deepest *Group reached so callers can
//     render that group's help alongside the error.
//   - parse failure of a flag token → ParseFlags' wrapped usage error.
//     Resolved is non-nil with Leaf+MergedFS already populated so callers can
//     render the resolved leaf's help (with its merged Flags block).
//   - presence of `--help` / `-h` anywhere → returns Resolved with ErrHelp;
//     callers (Dispatch) render help instead of running the leaf.
func Resolve(root *Group, args []string) (*Resolved, error) {
	if root == nil {
		return nil, fmt.Errorf("resolve: nil root")
	}

	// shapes is consulted ONLY by the walker to decide whether a flag token
	// consumes the next argv entry as its value. It is NOT used for parsing.
	shapes := flag.NewFlagSet("shapes", flag.ContinueOnError)
	shapes.SetOutput(io.Discard)
	if root.Flags != nil {
		root.Flags(shapes)
	}

	cur := root
	path := []*Group{root}
	var leaf Command = root
	helpRequested := false
	flagTokens := []string{}
	leafArgs := []string{}

	i := 0
	for i < len(args) {
		tok := args[i]
		if tok == "--" {
			// Pass `--` and everything after through to the leaf — stdlib
			// FlagSet.Parse strips the `--` and treats the rest as positional.
			leafArgs = append(leafArgs, args[i+1:]...)
			// Keep the `--` token in flagTokens so ParseFlags terminates flag
			// parsing at the right point.
			flagTokens = append(flagTokens, tok)
			break
		}
		name, _, hasEquals, isLong := parseFlagToken(tok)
		if isLong {
			if name == "help" || name == "h" {
				helpRequested = true
				flagTokens = append(flagTokens, tok)
				i++
				continue
			}
			flagTokens = append(flagTokens, tok)
			if !hasEquals && flagTakesValue(shapes, name) && i+1 < len(args) {
				flagTokens = append(flagTokens, args[i+1])
				i += 2
				continue
			}
			i++
			continue
		}

		// Positional. If we're still at a Group, try to descend by name.
		if cur != nil {
			if child, ok := cur.Children[tok]; ok {
				if grp, isGroup := child.(*Group); isGroup {
					cur = grp
					path = append(path, grp)
					leaf = grp
					if grp.Flags != nil {
						grp.Flags(shapes)
					}
					i++
					continue
				}
				// Leaf reached. Subsequent positionals belong to the leaf.
				cur = nil
				leaf = child
				if fl, ok := child.(Flagger); ok {
					addShapesGuarded(shapes, fl.Flags)
				}
				i++
				continue
			}
			return &Resolved{Leaf: cur, Path: path},
				fmt.Errorf("unknown subcommand %q: %w", tok, ErrUsage)
		}
		leafArgs = append(leafArgs, tok)
		i++
	}

	merged := buildMergedFlagSet(leaf, path)

	if helpRequested {
		// Don't try to parse — `--help` mid-args may sit before required
		// values. Caller renders help and returns ErrHelp.
		return &Resolved{Leaf: leaf, Path: path, MergedFS: merged, Args: leafArgs}, ErrHelp
	}

	if len(flagTokens) > 0 {
		if err := ParseFlags(merged, flagTokens); err != nil {
			// Return the partial Resolved so callers can render the resolved
			// leaf's help (and its merged Flags block) alongside the error.
			return &Resolved{Leaf: leaf, Path: path, MergedFS: merged, Args: leafArgs}, err
		}
		// ParseFlags' internal loop reseeds positional remainder via `--`.
		leafArgs = append(leafArgs, merged.Args()...)
	}

	return &Resolved{Leaf: leaf, Path: path, MergedFS: merged, Args: leafArgs}, nil
}

// flagTakesValue reports whether the flag named `name` on fs consumes the
// next argv token as its value. Bools don't; everything else does. Unknown
// flags default to "takes value" so we don't accidentally treat the next
// positional as a verb name (which would then likely fail unknown-verb anyway
// — better to error at parse time with a clearer message).
func flagTakesValue(fs *flag.FlagSet, name string) bool {
	f := fs.Lookup(name)
	if f == nil {
		return true
	}
	bf, ok := f.Value.(interface{ IsBoolFlag() bool })
	if !ok || !bf.IsBoolFlag() {
		return true
	}
	return false
}

// buildMergedFlagSet assembles the parse-time FlagSet in leaf-wins order.
// Leaf flags are registered first via their own Flagger.Flags so they own
// the names they care about. Each ancestor's Flags closure is then run on a
// private shadow set; non-clashing entries are copied onto merged with the
// shadow's flag.Value preserved (so the original target pointer is still the
// write target on Parse).
func buildMergedFlagSet(leaf Command, path []*Group) *flag.FlagSet {
	merged := flag.NewFlagSet("ana", flag.ContinueOnError)
	merged.SetOutput(io.Discard)
	// A *Group as "leaf" means the user stopped at a group prefix (`ana org`).
	// Don't call Flags on it via Flagger here; we still copy its Flags via
	// the path loop below.
	if _, isGroup := leaf.(*Group); !isGroup {
		if fl, ok := leaf.(Flagger); ok {
			fl.Flags(merged)
		}
	}
	for _, g := range path {
		if g.Flags == nil {
			continue
		}
		mergeShadow(merged, g.Flags)
	}
	return merged
}

// mergeShadow runs registrar against a private FlagSet and copies its flag
// declarations onto merged, skipping any name already present. The Value
// instance from the shadow is reused on merged so writes during Parse still
// land in the original target the closure bound.
func mergeShadow(merged *flag.FlagSet, registrar func(*flag.FlagSet)) {
	shadow := flag.NewFlagSet("shadow", flag.ContinueOnError)
	shadow.SetOutput(io.Discard)
	registrar(shadow)
	shadow.VisitAll(func(f *flag.Flag) {
		if merged.Lookup(f.Name) != nil {
			return
		}
		merged.Var(f.Value, f.Name, f.Usage)
	})
}

// addShapesGuarded mirrors mergeShadow but writes into the shapes set. Used
// when a leaf's Flagger flags need to influence the walker's value-consume
// decisions, but without panicking on duplicate names.
func addShapesGuarded(shapes *flag.FlagSet, registrar func(*flag.FlagSet)) {
	shadow := flag.NewFlagSet("shadow", flag.ContinueOnError)
	shadow.SetOutput(io.Discard)
	registrar(shadow)
	shadow.VisitAll(func(f *flag.Flag) {
		if shapes.Lookup(f.Name) != nil {
			return
		}
		shapes.Var(f.Value, f.Name, f.Usage)
	})
}

// globalFromFlagSet pulls the four well-known root persistent flags out of
// fs into a Global. Names match what the root group's Flags closure declares
// in cmd/ana/main.go. Missing names produce zero values so old tests that
// dispatch with a bare verb tree still work.
func globalFromFlagSet(fs *flag.FlagSet) Global {
	var g Global
	if f := fs.Lookup("json"); f != nil {
		g.JSON = strings.EqualFold(f.Value.String(), "true")
	}
	if f := fs.Lookup("endpoint"); f != nil {
		g.Endpoint = f.Value.String()
	}
	if f := fs.Lookup("token-file"); f != nil {
		g.TokenFile = f.Value.String()
	}
	if f := fs.Lookup("profile"); f != nil {
		g.Profile = f.Value.String()
	}
	return g
}

// Execute runs the leaf identified by Resolve against the given context and
// stdio. If Leaf is a *Group (the user typed only a group prefix) the
// group's help is printed and ErrHelp is returned. Otherwise the parsed
// merged FlagSet is plumbed through ctx via WithFlagSet (so leaves can fetch
// it for cli.RequireFlags / FlagWasSet) and Run is invoked with the leaf's
// positional args.
//
// Execute is the single chokepoint for the modern-CLI-convention guarantee:
// any error from the leaf that wraps ErrUsage but is NOT yet tagged
// ErrReported is rewritten to "<error>\n\n<help>" on stdio.Stderr (using the
// resolved leaf's own help block) and re-tagged with ErrReported so main()'s
// fallback printer does not double-emit. Callers therefore never need to
// repeat that wrap themselves.
//
// Execute does NOT call WithGlobal — the caller (Dispatch or cmd/ana) owns
// that decision so it can route any ancestor-bound Global pointer it cares
// to read.
func (r *Resolved) Execute(ctx context.Context, stdio IO) error {
	if g, ok := r.Leaf.(*Group); ok {
		fmt.Fprintln(stdio.Stdout, g.Help())
		return ErrHelp
	}
	if FlagSetFrom(ctx) == nil {
		ctx = WithFlagSet(ctx, r.MergedFS)
	}
	err := r.Leaf.Run(ctx, r.Args, stdio)
	if shouldAttachUsageHelp(err) {
		// Path[0] is the resolver's original root; Resolve always seeds it,
		// so the lookup is safe without a guard.
		ReportUsageError(r, r.Path[0], err, stdio.Stderr)
		return errors.Join(err, ErrReported)
	}
	return err
}

// flagSetKey is the unexported ctx key for a parsed FlagSet. Leaves that
// need post-parse access — for example to call cli.RequireFlags or
// cli.FlagWasSet — fetch it via FlagSetFrom.
type flagSetKey struct{}

// WithFlagSet attaches fs to ctx so leaves can fetch it via FlagSetFrom.
// Per stdlib context.WithValue contract, ctx must not be nil.
func WithFlagSet(ctx context.Context, fs *flag.FlagSet) context.Context {
	return context.WithValue(ctx, flagSetKey{}, fs)
}

// FlagSetFrom returns the FlagSet stashed by WithFlagSet, or nil if absent.
// Per stdlib context.Value semantics, ctx must not be nil.
func FlagSetFrom(ctx context.Context) *flag.FlagSet {
	fs, _ := ctx.Value(flagSetKey{}).(*flag.FlagSet)
	return fs
}
