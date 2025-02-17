package config

import (
	"fmt"
	"regexp"

	"github.com/apex/log"
	"github.com/vapor-ware/chart-releaser/pkg/strategies"
	"github.com/vapor-ware/chart-releaser/pkg/templates"
	u "github.com/vapor-ware/chart-releaser/pkg/utils"
	context "github.com/vapor-ware/chart-releaser/pkg/v1/ctx"
	"github.com/vapor-ware/chart-releaser/pkg/v1/utils"
)

// Stage for the "config" step of the update pipeline.
type Stage struct{}

// Name of the stage.
func (Stage) Name() string {
	return "config"
}

// String describes what the stage does.
func (Stage) String() string {
	return "loading context configuration"
}

// Run the operations defined for the stage.
func (Stage) Run(ctx *context.Context) error {
	// Update Strategy
	if err := loadUpdateStrategy(ctx); err != nil {
		return err
	}

	// Publish Strategy
	if err := loadPublishStrategy(ctx); err != nil {
		return err
	}

	// Template Strings
	if err := loadTemplateStrings(ctx); err != nil {
		return err
	}

	// Author
	log.Debug("loading author context")
	ctx.Author.Name = ctx.Config.Commit.Author.Name
	if ctx.Author.Name == "" {
		log.Debug("no commit author set, discovering from git config")
		name, err := u.GetGitUserName()
		if err != nil {
			if err := ctx.CheckDryRun(err); err != nil {
				log.WithError(err).Error("unable to determine committer name - must be set explicitly")
				return err
			}
			log.Info("dry-run: using stand-in for committer name")
			name = "dry-run-user"
		}
		ctx.Author.Name = name
	}
	ctx.Author.Email = ctx.Config.Commit.Author.Email
	if ctx.Author.Email == "" {
		log.Debug("no commit email set, discovering from git config")
		email, err := u.GetGitUserEmail()
		if err != nil {
			if err := ctx.CheckDryRun(err); err != nil {
				log.WithError(err).Error("unable to determine committer email - must be set explicitly")
				return err
			}
			log.Info("dry-run: using stand-in for committer email")
			email = "dry-run@commiter.dev"
		}
		ctx.Author.Email = email
	}

	// Chart
	log.Debug("loading chart context")
	ctx.Chart.Name = ctx.Config.Chart.Name
	ctx.Chart.SubPath = ctx.Config.Chart.Path

	// Repository
	log.Debug("loading repository context")
	repo, err := utils.ParseRepository(ctx.Config.Chart.Repo)
	if err != nil {
		return err
	}
	ctx.Repository = repo

	// Release
	log.Debug("loading release constraints")
	if err := loadReleaseConstraints(ctx); err != nil {
		return err
	}
	return nil
}

func loadUpdateStrategy(ctx *context.Context) error {
	log.Debug("loading upgrade strategy context")
	s := ctx.Config.Release.Strategy
	if s == "" {
		log.WithField("strategy", "default").Info("no release strategy configured, using default")
		s = "default"
	}
	releaseStrat, err := strategies.UpdateStrategyFromString(s)
	if err != nil {
		return err
	}
	ctx.UpdateStrategy = releaseStrat
	return nil
}

func loadPublishStrategy(ctx *context.Context) error {
	log.Debug("loading publish strategy context")
	if ctx.Config.Publish.Commit == nil && ctx.Config.Publish.PR == nil {
		log.WithField("default", strategies.PublishPullRequest).Debug("no publish config defined, using default publish strategy")
		ctx.PublishStrategy = strategies.PublishPullRequest
	} else if ctx.Config.Publish.Commit != nil {
		ctx.PublishStrategy = strategies.PublishCommit
	} else if ctx.Config.Publish.PR != nil {
		ctx.PublishStrategy = strategies.PublishPullRequest
	}
	return nil
}

func loadTemplateStrings(ctx *context.Context) error {
	if ctx.Config.Commit.Templates == nil {
		ctx.Release.ChartCommitMsg = templates.DefaultUpdateCommitMessage
		log.WithField("default", ctx.Release.ChartCommitMsg).Debug("using default commit message for updating Chart")

		ctx.Release.ExtrasCommitMsg = templates.DefaultExtrasCommitMessage
		log.WithField("default", ctx.Release.ExtrasCommitMsg).Debug("using default commit message for updating extra files")
	} else {
		ctx.Release.ChartCommitMsg = ctx.Config.Commit.Templates.Update
		if ctx.Release.ChartCommitMsg == "" {
			ctx.Release.ChartCommitMsg = templates.DefaultUpdateCommitMessage
			log.WithField("default", ctx.Release.ChartCommitMsg).Debug("using default commit message for updating Chart")
		}

		ctx.Release.ExtrasCommitMsg = ctx.Config.Commit.Templates.Extras
		if ctx.Release.ExtrasCommitMsg == "" {
			ctx.Release.ExtrasCommitMsg = templates.DefaultExtrasCommitMessage
			log.WithField("default", ctx.Release.ExtrasCommitMsg).Debug("using default commit message for updating extra files")
		}
	}

	switch ctx.PublishStrategy {
	case strategies.PublishCommit:
		ctx.Git.Ref = ctx.Config.Publish.Commit.Branch
		if ctx.Git.Ref == "" {
			ctx.Git.Ref = "master"
			log.WithField("default", ctx.Git.Ref).Debug("no publish commit branch defined, using default")
		}

		ctx.Git.Base = ctx.Config.Publish.Commit.Base
		if ctx.Git.Base == "" {
			ctx.Git.Base = "master"
			log.WithField("default", ctx.Git.Base).Debug("no publish commit base branch defined, using default")
		}

	case strategies.PublishPullRequest:
		ctx.Git.Ref = ctx.Config.Publish.PR.BranchTemplate
		if ctx.Git.Ref == "" {
			ctx.Git.Ref = templates.DefaultBranchName
			log.WithField("default", ctx.Git.Ref).Debug("no PR branch template defined, using default")
		}

		ctx.Git.Base = ctx.Config.Publish.PR.Base
		if ctx.Git.Base == "" {
			ctx.Git.Base = "master"
			log.WithField("default", ctx.Git.Base).Debug("no publish PR base branch defined, using default")
		}

		ctx.Release.PRTitle = ctx.Config.Publish.PR.TitleTemplate
		if ctx.Release.PRTitle == "" {
			ctx.Release.PRTitle = templates.DefaultPullRequestTitle
			log.WithField("default", ctx.Release.PRTitle).Debug("using default pull request title")
		}

		ctx.Release.PRBody = ctx.Config.Publish.PR.BodyTemplate
		if ctx.Release.PRBody == "" {
			ctx.Release.PRBody = templates.DefaultPullRequestBody
			log.WithField("default", ctx.Release.PRBody).Debug("using default pull request body")
		}

	default:
		return fmt.Errorf("unsupported publish strategy: %s", ctx.PublishStrategy)
	}
	return nil
}

func loadReleaseConstraints(ctx *context.Context) error {
	for _, match := range ctx.Config.Release.Matches {
		r, err := regexp.Compile(match)
		if err != nil {
			if err := ctx.CheckDryRun(err); err != nil {
				return err
			}
			log.Warnf("dry-run: failed to compile release match regex '%s', using stand-in", match)
			r = regexp.MustCompile("dry-run")
		}
		ctx.Release.Matches = append(ctx.Release.Matches, r)
	}

	for _, ignore := range ctx.Config.Release.Ignores {
		r, err := regexp.Compile(ignore)
		if err != nil {
			if err := ctx.CheckDryRun(err); err != nil {
				return err
			}
			log.Warnf("dry-run: failed to compile release ignore regex '%s', using stand-in", ignore)
			r = regexp.MustCompile("dry-run")
		}
		ctx.Release.Ignores = append(ctx.Release.Ignores, r)
	}
	return nil
}
