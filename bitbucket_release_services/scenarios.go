package bitbucket_release_services

import (
	"fmt"
	"github.com/pkg/errors"
	"github.com/sharovik/devbot/events/bitbucketrelease/bitbucketrelease_dto"
	"github.com/sharovik/devbot/internal/client"
	"github.com/sharovik/devbot/internal/container"
	"github.com/sharovik/devbot/internal/dto"
	"github.com/sharovik/devbot/internal/log"
	"time"
)

func MergeOnePullRequestScenario(message dto.BaseChatMessage, canBeMergedPullRequestList map[string]bitbucketrelease_dto.PullRequest) error {
	log.Logger().Debug().Msg("There is only 1 received pull-request. Trying to merge it.")
	newText, err := MergePullRequests(canBeMergedPullRequestList, client.StrategySquash)
	if err != nil {
		log.Logger().AddError(err).Msg("Failed to merge the pull-request")
		log.Logger().FinishMessage("Merge of received pull-requests")
		return err
	}

	SendMessageToTheChannel(message.Channel, fmt.Sprintf("%s\n", newText))
	log.Logger().FinishMessage("Merge of received pull-requests")
	return nil
}

func MergeMultiplePullRequestsScenario(message dto.BaseChatMessage, repository string, pullRequests map[string]bitbucketrelease_dto.PullRequest) error {
	//This is for multiple pull-requests links
	var (
		repositories                  = map[string]dto.BitBucketResponseBranchCreate{}
		workspace                     = ""
		releasePullRequestDescription = ""
		pullRequestsToMerge           = map[string]bitbucketrelease_dto.PullRequest{}
		releaseBranchName             = fmt.Sprintf("release/%s", time.Now().Format("2006.01.02"))
	)

	SendMessageToTheChannel(message.Channel, fmt.Sprintf("For repository `%s` we have more then 1 pull-request. I will create a release-branch.", repository))

	//In that case we have multiple pull-requests for that repository, so we have to create a release branch
	for _, pullRequest := range pullRequests {
		if workspace == "" {
			workspace = pullRequest.Workspace
		}

		releasePullRequestDescription += fmt.Sprintf("%s\n", pullRequest.Description)

		//If we don't have any created release branch for this repository the we need to create it
		if repositories[repository].Name == "" {
			branchResponse, err := container.C.BibBucketClient.CreateBranch(pullRequest.Workspace, pullRequest.RepositorySlug, releaseBranchName)
			if err != nil {
				log.Logger().AddError(err).Msg("Received an error during the release branch creation")
				return errors.Wrap(err, fmt.Sprintf("\nThe release-branch for repository %s cannot be created, because of `%s`", repository, err))
			}

			repositories[repository] = branchResponse
		}

		//We switch the destination of the pull-request to the release branch
		_, err := container.C.BibBucketClient.ChangePullRequestDestination(
			pullRequest.Workspace,
			pullRequest.RepositorySlug,
			pullRequest.ID,
			prepareReleaseTitle(pullRequest.Title),
			releaseBranchName)
		if err != nil {
			SendMessageToTheChannel(message.Channel, fmt.Sprintf("I've tried to switch the destination for pull-request #%d and I failed. Reason: `%s`\nNote! This pull-request will not be merged into release branch!", pullRequest.ID, err))
			log.Logger().AddError(err).Msg("Received an error during the branch destination switch")
			continue
		}

		pullRequestsToMerge[repository] = pullRequest
	}

	SendMessageToTheChannel(message.Channel, fmt.Sprintf("Trying to merge the %d pull-requests to the `%s` branch  of `%s` repository", len(pullRequests), releaseBranchName, repository))
	newText, err := MergePullRequests(pullRequestsToMerge, client.StrategySquash)
	if err != nil {
		log.Logger().AddError(err).Msg("Received error during multiple pull-request merge")
		log.Logger().FinishMessage("Merge of received pull-requests")
		return err
	}

	SendMessageToTheChannel(message.Channel, newText)

	//Now we need to create the pull-request
	pullRequestLink, err := createReleasePullRequest(workspace, repository, repositories[repository], releasePullRequestDescription)
	if err != nil {
		log.Logger().FinishMessage("Merge of received pull-requests")
		return errors.Wrap(err, fmt.Sprintf("\nI tried to create the release pull-request and I failed. Reason: %s", err))
	}

	SendMessageToTheChannel(message.Channel, fmt.Sprintf("\nPlease approve release pull-request: `%s`", pullRequestLink))
	return nil
}
