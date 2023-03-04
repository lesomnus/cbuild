package load

import (
	"context"
	"fmt"
	"strings"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/reference"
	"github.com/lesomnus/clade"
	"github.com/lesomnus/clade/plf"
	"github.com/lesomnus/pl"
	"github.com/rs/zerolog"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
)

func toString(value any) (string, bool) {
	switch v := value.(type) {
	case interface{ String() string }:
		return v.String(), true
	case string:
		return v, true

	default:
		return "", false
	}
}

func executeBeSingleString(executor *pl.Executor, pl *pl.Pl, data any) (string, error) {
	results, err := executor.Execute(pl, data)
	if err != nil {
		return "", err
	}
	if len(results) != 1 {
		return "", fmt.Errorf("expect result be sized 1 but was %d", len(results))
	}

	v, ok := toString(results[0])
	if !ok {
		return "", fmt.Errorf("expect result be string or stringer")
	}

	return v, nil
}

type Namespace interface {
	Repository(named reference.Named) (distribution.Repository, error)
}

type Expand struct {
	Registry Namespace
}

func (e *Expand) remoteTags(ctx context.Context, ref reference.Named) ([]string, error) {
	repo, err := e.Registry.Repository(ref)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	tags, err := repo.Tags(ctx).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get tags: %w", err)
	}

	return tags, nil
}

func (e *Expand) newTagsFunc(ctx context.Context, ref *clade.ImageReference, bg *clade.BuildGraph) func() ([]string, error) {
	return func() ([]string, error) {
		if tags, ok := bg.TagsByName(ref.Named); ok {
			return tags, nil
		}

		return e.remoteTags(ctx, ref)
	}
}

func (e *Expand) newExecutor(ctx context.Context, image *clade.Image, bg *clade.BuildGraph) *pl.Executor {
	l := zerolog.Ctx(ctx)

	executor := pl.NewExecutor()
	maps.Copy(executor.Funcs, plf.Funcs())
	executor.Convs.MergeWith(plf.Convs())
	executor.Funcs["log"] = func(vs ...string) []string {
		l.Info().Str("name", image.Name()).Msg(strings.Join(vs, ", "))
		return vs
	}
	executor.Funcs["tags"] = e.newTagsFunc(ctx, image.From.Primary, bg)
	executor.Funcs["tagsOf"] = func(ref string) ([]string, error) {
		named, err := reference.ParseNamed(ref)
		if err != nil {
			return nil, fmt.Errorf("reference must be fully named: %w", err)
		}

		if tags, ok := bg.TagsByName(named); ok {
			return tags, nil
		}

		return e.remoteTags(ctx, named)
	}

	return executor
}

func (e *Expand) Load(ctx context.Context, bg *clade.BuildGraph, ports []*clade.Port) error {
	dg := clade.NewDependencyGraph()
	for _, port := range ports {
		for _, image := range port.Images {
			dg.Put(image)
		}
	}

	snapshot := dg.Snapshot()
	names := maps.Keys(snapshot)
	slices.SortFunc(names, func(lhs string, rhs string) bool {
		return snapshot[lhs].Level < snapshot[rhs].Level
	})
	for _, name := range names {
		node, ok := dg.Get(name)
		if !ok {
			panic("node must be exists")
		}
		if len(node.Prev) == 0 {
			// Skip root nodes.
			continue
		}

		for _, image := range node.Value {
			resolved_images, err := e.Expand(ctx, image, bg)
			if err != nil {
				return fmt.Errorf(`expand "%s": %w`, name, err)
			}

			for _, resolved_image := range resolved_images {
				if _, err := bg.Put(resolved_image); err != nil {
					return fmt.Errorf(`put "%s" into build graph: %w`, resolved_image.String(), err)
				}
			}
		}
	}

	return nil
}

func (e *Expand) Expand(ctx context.Context, image *clade.Image, bg *clade.BuildGraph) ([]*clade.ResolvedImage, error) {
	executor := e.newExecutor(ctx, image, bg)

	// Executes `images[].from.tags`.
	from_results, err := executor.Execute((*pl.Pl)(image.From.Primary.Tag), nil)
	if err != nil {
		return nil, fmt.Errorf("from.tags: execute pipeline: %w", err)
	}

	resolved_images := make([]*clade.ResolvedImage, len(from_results))
	for i, result := range from_results {
		tag, ok := toString(result)
		if !ok {
			return nil, fmt.Errorf("from.tags: result must be string or stringer: %v", result)
		}

		primary_image := image.From.Primary
		tagged, err := reference.WithTag(primary_image.Named, tag)
		if err != nil {
			return nil, fmt.Errorf(`invalid tag "%s" of image "%s": %w`, tag, image.Name(), err)
		}

		if primary_image.Alias == "" {
			primary_image.Alias = "BASE"
		}

		resolved_images[i] = &clade.ResolvedImage{
			From: &clade.ResolvedBaseImage{
				Primary: clade.ResolvedImageReference{
					NamedTagged: tagged,
					Alias:       primary_image.Alias,
				},
			},
			Skip: false,
		}
	}

	// Executes `images[].from.with[].tag`.
	for i, result := range from_results {
		resolved_image := resolved_images[i]
		if len(image.From.Secondaries) == 0 {
			continue
		}

		resolved_image.From.Secondaries = make([]clade.ResolvedImageReference, len(image.From.Secondaries))
		for j, ref := range image.From.Secondaries {
			if err := func() error {
				executor.Funcs["tags"] = e.newTagsFunc(ctx, ref, bg)

				tag, err := executeBeSingleString(executor, (*pl.Pl)(ref.Tag), result)
				if err != nil {
					return fmt.Errorf("execute: ")
				}

				tagged, err := reference.WithTag(ref.Named, tag)
				if err != nil {
					return fmt.Errorf(`"%s": %w`, tag, err)
				}

				resolved_image.From.Secondaries[j] = clade.ResolvedImageReference{
					NamedTagged: tagged,
					Alias:       ref.Alias,
				}
				return nil
			}(); err != nil {
				return nil, fmt.Errorf("from.with[%d].tag: %w", j, err)
			}
		}
	}

	// No `tags` function for rest pipelines.
	delete(executor.Funcs, "tags")

	// Executes `image[].tags`.
	for i, result := range from_results {
		tags := make([]string, len(image.Tags))
		for j, tag := range image.Tags {
			v, err := executeBeSingleString(executor, (*pl.Pl)(&tag), result)
			if err != nil {
				return nil, fmt.Errorf("tags[%d]: %w", j, err)
			}

			tags[j] = v
		}

		resolved_images[i].Tags = tags
	}

	// Executes `image[].args`.
	for i, result := range from_results {
		args := make(map[string]string)
		for key, arg := range image.Args {
			v, err := executeBeSingleString(executor, (*pl.Pl)(&arg), result)
			if err != nil {
				return nil, fmt.Errorf("args[%s]: %w", key, err)
			}

			args[key] = v
		}

		resolved_images[i].Args = args
	}

	for i := range resolved_images {
		resolved_images[i].Named = image.Named
		resolved_images[i].Skip = image.Skip
		resolved_images[i].Dockerfile = image.Dockerfile
		resolved_images[i].ContextPath = image.ContextPath
		resolved_images[i].Platform = image.Platform
	}

	// TODO: make it as functions.
	for i := 0; i < len(resolved_images); i++ {
		for j := i + 1; j < len(resolved_images); j++ {
			clade.DeduplicateBySemver(&resolved_images[i].Tags, &resolved_images[j].Tags)
		}
	}

	// Remove empty.
	head := 0
	last := len(resolved_images) - 1
	for {
		if head > last {
			break
		}

		image := resolved_images[head]
		if len(image.Tags) > 0 {
			head++
			continue
		}

		resolved_images[last], resolved_images[head] = resolved_images[head], resolved_images[last]
		last--
	}
	resolved_images = resolved_images[:last+1]

	// Test if duplicated tags are exist.
	for i := 0; i < len(resolved_images); i++ {
		for j := i + 1; j < len(resolved_images); j++ {
			for _, lhs := range resolved_images[i].Tags {
				for _, rhs := range resolved_images[j].Tags {
					if lhs == rhs {
						return nil, fmt.Errorf("%s: tag is duplicated: %s", image.From.Primary.String(), lhs)
					}
				}
			}
		}
	}

	return resolved_images, nil
}
