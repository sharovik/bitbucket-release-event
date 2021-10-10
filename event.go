package bitbucketrelease

import (
	"fmt"
	"github.com/sharovik/devbot/events/bitbucketrelease/bitbucket_release_services"
	"github.com/sharovik/devbot/events/bitbucketrelease/bitbucketrelease_dto"
	"github.com/sharovik/devbot/internal/container"
	"github.com/sharovik/devbot/internal/database"
	"github.com/sharovik/devbot/internal/dto"
	"github.com/sharovik/devbot/internal/helper"
	"github.com/sharovik/devbot/internal/log"
)

//EventName the name of the event
const (
	EventName         = "bitbucket_release"
	EventVersion      = "1.1.0"
	pullRequestsRegex = `(?m)https:\/\/bitbucket.org\/(?P<workspace>.+)\/(?P<repository_slug>.+)\/pull-requests\/(?P<pull_request_id>\d+)`
	helpMessage       = "Send me message ```bb release {links-to-pull-requests}``` with the links to the bitbucket pull-requests instead of `{links-to-pull-requests}`.\nExample: bb release https://bitbucket.org/mywork/my-test-repository/pull-requests/1"

	pullRequestStringAnswer   = "I found the next pull-requests:\n"
	noPullRequestStringAnswer = `I can't find any pull-request in your message`

	pullRequestStateOpen   = "OPEN"
)

//ReceivedPullRequests struct for pull-requests list
type ReceivedPullRequests struct {
	Items []bitbucketrelease_dto.PullRequest
}

//PullRequest the pull-request item
type PullRequest struct {
	ID             int64
	RepositorySlug string
	BranchName     string
	Workspace      string
	Title          string
	Description    string
}

//ReleaseEvent event of BitBucket release
type ReleaseEvent struct {
	EventName string
}

//Event - object which is ready to use
var (
	Event = ReleaseEvent{
	EventName: EventName,
}
	m = []database.BaseMigrationInterface{
		UpdateReleaseTriggerMigration{},
	}
)

type failedToMerge struct {
	Reason string
	Info   dto.BitBucketPullRequestInfoResponse
	Error  error
	PullRequest bitbucketrelease_dto.PullRequest
}

//Install method for installation of event
func (e ReleaseEvent) Install() error {
	log.Logger().Debug().
		Str("event_name", EventName).
		Str("event_version", EventVersion).
    Msg("Start event Install")

	return container.C.Dictionary.InstallEvent(
		EventName,                              //We specify the event name which will be used for scenario generation
		EventVersion,                           //This will be set during the event creation
		"release",                           //Actual question, which system will wait and which will trigger our event
		"Ok, give me a minute", //Answer which will be used by the bot
		"(?i)bb release",                       //Optional field. This is regular expression which can be used for question parsing.
		"", 
	)
}

//Update the method applies updates
func (e ReleaseEvent) Update() error {
	for _, migration := range m {
		container.C.MigrationService.SetMigration(migration)
	}

	return container.C.MigrationService.RunMigrations()
}

//Execute the main method for event execution
func (ReleaseEvent) Execute(message dto.BaseChatMessage) (dto.BaseChatMessage, error) {
	var answer = message

	isHelpAnswerTriggered, err := helper.HelpMessageShouldBeTriggered(answer.OriginalMessage.Text)
	if err != nil {
		log.Logger().Warn().Err(err).Msg("Something went wrong with help message parsing")
	}

	if isHelpAnswerTriggered {
		answer.Text = helpMessage
		return answer, nil
	}

	//First we need to find all the pull-requests in received message
	foundPullRequests := findAllPullRequestsInText(pullRequestsRegex, answer.OriginalMessage.Text)

	//We prepare the text, where we define all the pull-requests which we found in the received message
	answer.Text = receivedPullRequestsText(foundPullRequests)

	//Next step is a pull-request statuses check
	canBeMergedPullRequestsList, canBeMergedByRepository, failedPullRequests := checkPullRequests(foundPullRequests.Items)

	//When we have failed pull-requests, we filter them out
	if len(failedPullRequests) > 0 {
		filterOutFailedRepositories(failedPullRequests, canBeMergedPullRequestsList, canBeMergedByRepository)
	}

	//We generate text for pull-requests which cannot be merged
	if len(failedPullRequests) > 0 {
		bitbucket_release_services.SendMessageToTheChannel(message.Channel, failedPullRequestsText(failedPullRequests))
	}

	bitbucket_release_services.SendMessageToTheChannel(message.Channel, canBeMergedPullRequestsText(canBeMergedPullRequestsList))

	if len(canBeMergedByRepository) == 0 {
		answer.Text += fmt.Sprintf("\nNothing to release")
		return answer, nil
	}

	if err := releaseThePullRequests(message, canBeMergedPullRequestsList, canBeMergedByRepository); err != nil {
		return answer, err
	}

	answer.Text += fmt.Sprintf("Done")

	if container.C.Config.BitBucketConfig.ReleaseChannelMessageEnabled && container.C.Config.BitBucketConfig.ReleaseChannel != "" {
		log.Logger().Debug().
			Str("channel", container.C.Config.BitBucketConfig.ReleaseChannel).
			Msg("Send release-confirmation message")

		bitbucket_release_services.SendMessageToTheChannel(container.C.Config.BitBucketConfig.ReleaseChannel, fmt.Sprintf("There were release triggered by <@%s>!", answer.OriginalMessage.User))
	}

	return answer, nil
}
