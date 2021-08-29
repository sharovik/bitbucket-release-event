package bitbucketrelease

import (
	"fmt"
	"github.com/sharovik/devbot/events/bitbucketrelease/bitbucket_release_services"
	"github.com/sharovik/devbot/events/bitbucketrelease/bitbucketrelease_dto"
	"regexp"
	"strconv"
	"strings"

	"github.com/sharovik/devbot/internal/container"
	"github.com/sharovik/devbot/internal/dto"
	"github.com/sharovik/devbot/internal/log"
)

func failedPullRequestsText(failedPullRequests map[string]failedToMerge) string {
	if len(failedPullRequests) == 0 {
		return "All pull-requests are ready for merge! This is awesome!"
	}

	var text = "These pull-requests cannot be merged:\n"

	for pullRequest, reason := range failedPullRequests {
		text += fmt.Sprintf("%s - %s \n", pullRequest, reason.Reason)
	}

	return text
}

func canBeMergedPullRequestsText(canBeMerged map[string]bitbucketrelease_dto.PullRequest) string {
	if len(canBeMerged) == 0 {
		return "There is no pull-requests, which can be merged."
	}

	var text = "From received pull-requests, next are good to go:\n"

	for pullRequestURL, pullRequest := range canBeMerged {
		text += fmt.Sprintf("[#%d] %s \n", pullRequest.ID, pullRequestURL)
	}

	return text
}

func checkPullRequests(items []bitbucketrelease_dto.PullRequest) (map[string]bitbucketrelease_dto.PullRequest, map[string]map[string]bitbucketrelease_dto.PullRequest, map[string]failedToMerge) {
	var (
		failedPullRequests         = make(map[string]failedToMerge)
		canBeMergedPullRequestList = make(map[string]bitbucketrelease_dto.PullRequest)
		canBeMergedByRepository    = make(map[string]map[string]bitbucketrelease_dto.PullRequest)
	)
	for _, pullRequest := range items {
		cleanPullRequestURL := fmt.Sprintf("https://bitbucket.org/%s/%s/pull-requests/%d", pullRequest.Workspace, pullRequest.RepositorySlug, pullRequest.ID)
		info, err := container.C.BibBucketClient.PullRequestInfo(pullRequest.Workspace, pullRequest.RepositorySlug, pullRequest.ID)
		if err != nil {
			failedPullRequests[cleanPullRequestURL] = failedToMerge{
				Reason: err.Error(),
				Info:   info,
				Error:  err,
				PullRequest: pullRequest,
			}

			continue
		}

		replacer := strings.NewReplacer("\\", "")
		pullRequest.Title = info.Title
		pullRequest.BranchName = info.Source.Branch.Name
		pullRequest.RepositorySlug = info.Source.Repository.Name
		pullRequest.Description = replacer.Replace(info.Description)

		cleanPullRequestURL = fmt.Sprintf("https://bitbucket.org/%s/%s/pull-requests/%d", pullRequest.Workspace, pullRequest.RepositorySlug, pullRequest.ID)

		if !isPullRequestAlreadyMerged(info) {
			failedPullRequests[cleanPullRequestURL] = failedToMerge{
				Reason: fmt.Sprintf("The state should be %s, instead of it %s received.", pullRequestStateOpen, info.State),
				Info:   info,
				Error:  nil,
				PullRequest: pullRequest,
			}

			continue
		}

		isPullRequestApprovedByReviewers := isApprovedByReviewers(info)
		if !isPullRequestApprovedByReviewers {
			failedPullRequests[cleanPullRequestURL] = failedToMerge{
				Reason: "Not all reviewers approved the change.",
				Info:   info,
				Error:  nil,
				PullRequest: pullRequest,
			}

			continue
		}

		log.Logger().Debug().
			Interface("pull_request", pullRequest).
			Msg("The pull-request can be merged.")

		canBeMergedPullRequestList[cleanPullRequestURL] = pullRequest

		if canBeMergedByRepository[pullRequest.RepositorySlug] == nil {
			canBeMergedByRepository[pullRequest.RepositorySlug] = make(map[string]bitbucketrelease_dto.PullRequest)
		}

		canBeMergedByRepository[pullRequest.RepositorySlug][pullRequest.Title] = pullRequest
	}

	return canBeMergedPullRequestList, canBeMergedByRepository, failedPullRequests
}

func releaseThePullRequests(message dto.BaseChatMessage, canBeMergedPullRequestList map[string]bitbucketrelease_dto.PullRequest, canBeMergedByRepository map[string]map[string]bitbucketrelease_dto.PullRequest) error {
	log.Logger().StartMessage("Merge of received pull-requests")

	//In case when we have only one pull-request we will merge it straight to the main branch
	if len(canBeMergedPullRequestList) == 1 {
		bitbucket_release_services.SendMessageToTheChannel(message.Channel, "We have only one pull-request, so I will try to merge it directly to the main branch.")
		return bitbucket_release_services.MergeOnePullRequestScenario(message, canBeMergedPullRequestList)
	}

	//Here we take sorted by repository pull-requests and trying to merge them into main or release branch.
	//We go in for loop into each repository and check how many pull-requests do we have there.
	//If only one, then we merge it into main branch, otherwise we create release branch for selected repository,
	//switch direction of the pull-requests to that release branch and merge all of them.
	for repository, pullRequests := range canBeMergedByRepository {
		//Well, in that case we have only one pull-request so we merge it into main branch
		if len(pullRequests) == 1 {
			log.Logger().Debug().Str("repository", repository).Msg("Only one pull-request received for selected repository")
			bitbucket_release_services.SendMessageToTheChannel(message.Channel, fmt.Sprintf("There is only one pull-request for repository `%s`.", repository))
			err := bitbucket_release_services.MergeOnePullRequestScenario(message, canBeMergedPullRequestList)
			if err != nil {
				log.Logger().AddError(err).Msg("Received error during pull-request merge")
			}

			continue
		}

		if err := bitbucket_release_services.MergeMultiplePullRequestsScenario(message, repository, pullRequests); err != nil {
			log.Logger().AddError(err).Msg("Failed to trigger multiple pull-requests scenario")
			bitbucket_release_services.SendMessageToTheChannel(message.Channel, fmt.Sprintf("Failed to merge: `%s`", err.Error()))
			continue
		}
	}

	log.Logger().FinishMessage("Merge of received pull-requests")
	return nil
}

func isApprovedByReviewers(info dto.BitBucketPullRequestInfoResponse) bool {
	requiredReviewers := container.C.Config.BitBucketConfig.RequiredReviewers
	if len(requiredReviewers) == 0 {
		return true
	}

	for _, user := range info.Participants {
		if !user.Approved {
			return false
		}
	}

	return true
}

func isPullRequestAlreadyMerged(info dto.BitBucketPullRequestInfoResponse) bool {
	if info.State == pullRequestStateOpen {
		return true
	}

	return false
}

func receivedPullRequestsText(foundPullRequests ReceivedPullRequests) string {

	if len(foundPullRequests.Items) == 0 {
		return noPullRequestStringAnswer
	}

	var pullRequestsString = pullRequestStringAnswer
	for _, item := range foundPullRequests.Items {
		pullRequestsString = pullRequestsString + fmt.Sprintf("Pull-request #%d\n", item.ID)
	}

	return pullRequestsString
}

func findAllPullRequestsInText(regex string, subject string) ReceivedPullRequests {
	re, err := regexp.Compile(regex)

	if err != nil {
		log.Logger().AddError(err).Msg("Error during the Find Matches operation")
		return ReceivedPullRequests{}
	}

	matches := re.FindAllStringSubmatch(subject, -1)
	result := ReceivedPullRequests{}

	if len(matches) == 0 {
		return result
	}

	for _, id := range matches {
		if id[1] != "" {
			item := bitbucketrelease_dto.PullRequest{}
			item.Workspace = id[1]
			item.RepositorySlug = id[2]
			item.ID, err = strconv.ParseInt(id[3], 10, 64)
			if err != nil {
				log.Logger().AddError(err).
					Interface("matches", matches).
					Msg("Error during pull-request ID parsing")
				return ReceivedPullRequests{}
			}

			result.Items = append(result.Items, item)
		}
	}

	return result
}

func filterOutFailedRepositories(failedPullRequests map[string]failedToMerge, canBeMergedPullRequestsList map[string]bitbucketrelease_dto.PullRequest, canBeMergedByRepository map[string]map[string]bitbucketrelease_dto.PullRequest) (map[string]bitbucketrelease_dto.PullRequest, map[string]map[string]bitbucketrelease_dto.PullRequest) {
	for url, pullRequest := range failedPullRequests {
		delete(canBeMergedPullRequestsList, url)
		if len(canBeMergedByRepository[pullRequest.PullRequest.RepositorySlug]) == 1 {
			delete(canBeMergedByRepository, pullRequest.PullRequest.RepositorySlug)
		}
	}

	return canBeMergedPullRequestsList, canBeMergedByRepository
}