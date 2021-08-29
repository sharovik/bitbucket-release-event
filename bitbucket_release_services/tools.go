package bitbucket_release_services

import (
	"fmt"
	"github.com/pkg/errors"
	"github.com/sharovik/devbot/events/bitbucketrelease/bitbucketrelease_dto"
	"github.com/sharovik/devbot/internal/client"
	"github.com/sharovik/devbot/internal/container"
	"github.com/sharovik/devbot/internal/dto"
	"github.com/sharovik/devbot/internal/helper"
	"github.com/sharovik/devbot/internal/log"
	"strings"
)

func MergePullRequests(pullRequests map[string]bitbucketrelease_dto.PullRequest, strategy string) (string, error) {
	var (
		releaseText     string
		repository      = ""
		lastPullRequest = bitbucketrelease_dto.PullRequest{}
	)

	for _, pullRequest := range pullRequests {
		lastPullRequest = pullRequest

		if repository == "" {
			repository = pullRequest.RepositorySlug
		}

		if isReleaseBranchName(pullRequest.BranchName) {
			strategy = client.StrategyMerge
			releaseText += fmt.Sprintf("I merge `#%d` pull-request using `merge` strategy, because it is a release pull-request.\n", pullRequest.ID)
		}

		response, err := container.C.BibBucketClient.MergePullRequest(pullRequest.Workspace, pullRequest.RepositorySlug, pullRequest.ID, pullRequest.Description, strategy)
		if err != nil {
			releaseText += fmt.Sprintf("I cannot merge the pull-request #%d because of error `%s`", pullRequest.ID, err.Error())
			log.Logger().Info().
				Interface("response", response).
				Str("repository", repository).
				Err(err).
				Int64("pull_request_id", pullRequest.ID).
				Msg("Failed to merge pull-request")
			return releaseText, err
		}

		log.Logger().Info().
			Interface("response", response).
			Int64("pull_request_id", pullRequest.ID).
			Msg("Merged pull-request")
	}

	if len(pullRequests) == 1 {
		releaseText += fmt.Sprintf("\nI merged pull-request #`%d` into destination branch of repository `%s` :)", lastPullRequest.ID, repository)
		return releaseText, nil
	}

	releaseText += fmt.Sprintf("\nI merged all pull-requests for repository `%s` into destination branch :)", repository)

	return releaseText, nil
}

func isReleaseBranchName(branchName string) bool {
	found, err := helper.IsFoundMatches("(?i)^(release\\/\\w+)", branchName)
	if err != nil {
		log.Logger().AddError(err).
			Str("branch", branchName).
			Msg("Failed to find the release-branch matches in the branch name.")
	}

	return found
}

func prepareReleaseTitle(currentTitle string) string {
	if !strings.Contains(currentTitle, "[PREPARED-FOR-RELEASE]") {
		return fmt.Sprintf("[PREPARED-FOR-RELEASE] %s", currentTitle)
	}

	return currentTitle
}

func createReleasePullRequest(workspace string, repository string, releaseBranch dto.BitBucketResponseBranchCreate, description string) (string, error) {
	bitBucketPullRequestCreate := dto.BitBucketRequestPullRequestCreate{
		Title:       "Release pull-request",
		Description: description,
		Source: dto.BitBucketPullRequestDestination{
			Branch: dto.BitBucketPullRequestDestinationBranch{
				Name: releaseBranch.Name,
			},
		},
	}

	//Append reviewers to this pull-request except the author of the branch(which is current user)
	for _, reviewerUuID := range container.C.Config.BitBucketConfig.RequiredReviewers {
		if reviewerUuID.UUID != container.C.Config.BitBucketConfig.CurrentUserUUID {
			bitBucketPullRequestCreate.Reviewers = append(bitBucketPullRequestCreate.Reviewers, dto.BitBucketReviewer{UUID: reviewerUuID.UUID})
		}
	}

	response, err := container.C.BibBucketClient.CreatePullRequest(workspace, repository, bitBucketPullRequestCreate)
	if err != nil {
		return "", err
	}

	if response.Links.HTML.Href == "" {
		log.Logger().Warn().Interface("response", response).Msg("There is no pull-request link in response.")
		return "", errors.New("The pull-request link was not found in the response. ")
	}

	return response.Links.HTML.Href, nil
}
