package bitbucketrelease

import (
	"fmt"
	"github.com/sharovik/devbot/events/bitbucketrelease/bitbucket_release_services"
	"github.com/sharovik/devbot/events/bitbucketrelease/bitbucketrelease_dto"
	"github.com/sharovik/devbot/internal/container"
	"github.com/sharovik/devbot/internal/database"
	"github.com/sharovik/devbot/internal/dto"
	"github.com/sharovik/devbot/internal/log"
)

// EventName the name of the event
const (
	EventName         = "bitbucket_release"
	EventVersion      = "2.0.0"
	pullRequestsRegex = `(?m)https:\/\/bitbucket.org\/(?P<workspace>.+)\/(?P<repository_slug>.+)\/pull-requests\/(?P<pull_request_id>\d+)`
	helpMessage       = "Send me message ```release {links-to-pull-requests}``` with the links to the bitbucket pull-requests instead of `{links-to-pull-requests}`.\nExample: bb release https://bitbucket.org/mywork/my-test-repository/pull-requests/1"

	pullRequestStringAnswer   = "I found the next pull-requests:\n"
	noPullRequestStringAnswer = `I can't find any pull-request in your message`

	pullRequestStateOpen = "OPEN"
)

// ReceivedPullRequests struct for pull-requests list
type ReceivedPullRequests struct {
	Items []bitbucketrelease_dto.PullRequest
}

// PullRequest the pull-request item
type PullRequest struct {
	ID             int64
	RepositorySlug string
	BranchName     string
	Workspace      string
	Title          string
	Description    string
}

// EventStruct event of BitBucket release
type EventStruct struct {
}

// Event - object which is ready to use
var (
	Event = EventStruct{}
	m     = []database.BaseMigrationInterface{}
)

type failedToMerge struct {
	Reason      string
	Info        dto.BitBucketPullRequestInfoResponse
	Error       error
	PullRequest bitbucketrelease_dto.PullRequest
}

func (e EventStruct) Help() string {
	return helpMessage
}

func (e EventStruct) Alias() string {
	return EventName
}

// Install method for installation of event
func (e EventStruct) Install() error {
	log.Logger().Debug().
		Str("event_name", EventName).
		Str("event_version", EventVersion).
		Msg("Triggered event installation")

	if err := container.C.Dictionary.InstallNewEventScenario(database.EventScenario{
		EventName:    EventName,
		EventVersion: EventVersion,
		Questions: []database.Question{
			{
				Question:      "release",
				QuestionRegex: "(?i)(release)",
				Answer:        "Ok, give me a minute",
			},
			{
				Question:      "bb release",
				QuestionRegex: "(?i)(bb release)",
				Answer:        "Ok, give me a minute",
			},
		},
	}); err != nil {
		return err
	}

	return nil
}

// Update the method applies updates
func (e EventStruct) Update() error {
	for _, migration := range m {
		container.C.MigrationService.SetMigration(migration)
	}

	return container.C.MigrationService.RunMigrations()
}

// Execute the main method for event execution
func (EventStruct) Execute(message dto.BaseChatMessage) (dto.BaseChatMessage, error) {
	var answer = message

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
		answer.Text += "\nNothing to release"
		return answer, nil
	}

	if err := releaseThePullRequests(message, canBeMergedPullRequestsList, canBeMergedByRepository); err != nil {
		return answer, err
	}

	answer.Text += "Done"

	if container.C.Config.BitBucketConfig.ReleaseChannelMessageEnabled && container.C.Config.BitBucketConfig.ReleaseChannel != "" {
		log.Logger().Debug().
			Str("channel", container.C.Config.BitBucketConfig.ReleaseChannel).
			Msg("Send release-confirmation message")

		bitbucket_release_services.SendMessageToTheChannel(container.C.Config.BitBucketConfig.ReleaseChannel, fmt.Sprintf("There were release triggered by <@%s>!", answer.OriginalMessage.User))
	}

	return answer, nil
}
