package bitbucketrelease

import (
	"fmt"
	"github.com/sharovik/devbot/internal/helper"
	"time"

	"github.com/sharovik/devbot/internal/container"
	"github.com/sharovik/devbot/internal/dto"
	"github.com/sharovik/devbot/internal/log"
)

//EventName the name of the event
const (
	EventName         = "bitbucket_release"
	EventVersion      = "1.0.1"
	pullRequestsRegex = `(?m)https:\/\/bitbucket.org\/(?P<workspace>.+)\/(?P<repository_slug>.+)\/pull-requests\/(?P<pull_request_id>\d+)`
	helpMessage       = "Send me message ```bb release {links-to-pull-requests}```."

	pullRequestStringAnswer   = "I found the next pull-requests:\n"
	noPullRequestStringAnswer = `I can't find any pull-request in your message`

	pullRequestStateOpen   = "OPEN"
	pullRequestStateMerged = "MERGED"
)

//ReceivedPullRequests struct for pull-requests list
type ReceivedPullRequests struct {
	Items []PullRequest
}

//PullRequest the pull-request item
type PullRequest struct {
	ID             int64
	RepositorySlug string
	Workspace      string
	Title          string
	Description    string
}

//ReleaseEvent event of BitBucket release
type ReleaseEvent struct {
	EventName string
}

//Event - object which is ready to use
var Event = ReleaseEvent{
	EventName: EventName,
}

type failedToMerge struct {
	Reason string
	Info   dto.BitBucketPullRequestInfoResponse
	Error  error
}

//Install method for installation of event
func (e ReleaseEvent) Install() error {
	log.Logger().Debug().
		Str("event_name", EventName).
		Str("event_version", EventVersion).
		Msg("Triggered event installation")

	return container.C.Dictionary.InstallEvent(
		EventName,      //We specify the event name which will be used for scenario generation
		EventVersion,   //This will be set during the event creation
		"bb release", //Actual question, which system will wait and which will trigger our event
		"Ok, let me check these pull-requests", //Answer which will be used by the bot
		"(?i)bb release", //Optional field. This is regular expression which can be used for question parsing.
		"", //Optional field. This is a regex group and it can be used for parsing the match group from the regexp result
	)
}

//Update the method applies updates
func (e ReleaseEvent) Update() error {
	return nil
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

	//We generate text for pull-requests which cannot be merged
	if len(canBeMergedPullRequestsList) == 0 {
		answer.Text += fmt.Sprintf("\n%s", failedPullRequestsText(failedPullRequests))
	}

	answer.Text += fmt.Sprintf("\n%s", canBeMergedPullRequestsText(canBeMergedPullRequestsList))

	if len(canBeMergedByRepository) == 0 {
		answer.Text += fmt.Sprintf("\nNothing to release")
		return answer, nil
	}

	resultText, err := releaseThePullRequests(canBeMergedPullRequestsList, canBeMergedByRepository)
	if err != nil {
		answer.Text += resultText
		return answer, err
	}

	answer.Text += fmt.Sprintf("\n%s", resultText)

	if container.C.Config.BitBucketConfig.ReleaseChannelMessageEnabled && container.C.Config.BitBucketConfig.ReleaseChannel != "" {
		log.Logger().Debug().
			Str("channel", container.C.Config.BitBucketConfig.ReleaseChannel).
			Msg("Send release-confirmation message")

		response, statusCode, err := container.C.SlackClient.SendMessage(dto.SlackRequestChatPostMessage{
			Channel:           container.C.Config.BitBucketConfig.ReleaseChannel,
			Text:              fmt.Sprintf("There were release triggered by <@%s>. %s", answer.OriginalMessage.User, resultText),
			AsUser:            true,
			Ts:                time.Time{},
			DictionaryMessage: dto.DictionaryMessage{},
			OriginalMessage:   dto.SlackResponseEventMessage{},
		})

		if err != nil {
			log.Logger().AddError(err).
				Interface("response", response).
				Interface("status", statusCode).
				Msg("Failed to sent answer message")
		}
	}

	return answer, nil
}
