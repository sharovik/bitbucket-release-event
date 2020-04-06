package bitbucket_release

import (
	"errors"
	"os"
	"path"
	"runtime"
	"testing"

	"github.com/sharovik/devbot/internal/config"
	"github.com/sharovik/devbot/internal/container"
	"github.com/sharovik/devbot/internal/dto"
	"github.com/sharovik/devbot/internal/log"
	mock "github.com/sharovik/devbot/test/mock/client"
	"github.com/stretchr/testify/assert"
)

func init() {
	//We switch pointer to the root directory for control the path from which we need to generate test-data file-paths
	_, filename, _, _ := runtime.Caller(0)
	dir := path.Join(path.Dir(filename), "../../")
	_ = os.Chdir(dir)
	log.Init(log.Config(container.C.Config))
}

//@todo: cover all checks
func TestBitBucketReleaseEvent_Execute_NoReviewers(t *testing.T) {
	container.C.Config.BitBucketConfig.RequiredReviewers = []config.BitBucketReviewer{
		{
			UUID:     "{test-uid}",
			SlackUID: "TESTSLACKID",
		},
		{
			UUID:     "{test-second-uid}",
			SlackUID: "TESTSECONDSLACKID",
		},
	}

	//PullRequest status OPEN but no participants
	container.C.BibBucketClient = &mock.MockedBitBucketClient{
		IsTokenInvalid: true,
		PullRequestInfoResponse: dto.BitBucketPullRequestInfoResponse{
			Title:        "Some title",
			Description:  "Feature;Some task description;\\(https://some-url.net/browse/error-502\\);JohnDoeProject",
			State:        pullRequestStateOpen,
			Participants: []dto.Participant{},
		},
	}

	var msg = dto.SlackRequestChatPostMessage{
		OriginalMessage: dto.SlackResponseEventMessage{
			Text: `release these pull-requests to production: https://bitbucket.org/john/test-repo/pull-requests/1/testing-pr-flow`,
		},
	}

	answer, err := Event.Execute(msg)
	assert.NoError(t, err)

	expectedText := "I found the next pull-requests:\nPull-request #1 [repository: test-repo]\n\nThese pull-requests cannot be merged:\nhttps://bitbucket.org/john/test-repo/pull-requests/1 - One of the required reviewers (<@TESTSLACKID>, <@TESTSECONDSLACKID>) was not found in the reviewers list. \n\nThere is no pull-requests, which can be merged.\nNothing to release"
	assert.Equal(t, expectedText, answer.Text)
}

func TestBitBucketReleaseEvent_Execute_HasReviewersButNotApproved(t *testing.T) {
	container.C.Config.BitBucketConfig.RequiredReviewers = []config.BitBucketReviewer{
		{
			UUID:     "{test-uid}",
			SlackUID: "TESTSLACKID",
		},
		{
			UUID:     "{test-second-uid}",
			SlackUID: "TESTSECONDSLACKID",
		},
	}

	//PullRequest status OPEN but no participants
	container.C.BibBucketClient = &mock.MockedBitBucketClient{
		IsTokenInvalid: true,
		PullRequestInfoResponse: dto.BitBucketPullRequestInfoResponse{
			Title:       "Some title",
			Description: "Feature;Some task description;\\(https://some-url.net/browse/error-502\\);JohnDoeProject",
			State:       pullRequestStateOpen,
			Participants: []dto.Participant{
				dto.Participant{
					User: dto.ParticipantUser{
						UUID: "{test-uid}",
					},
					Approved: false,
				},
			},
		},
	}

	var msg = dto.SlackRequestChatPostMessage{
		OriginalMessage: dto.SlackResponseEventMessage{
			Text: `release these pull-requests to production: https://bitbucket.org/john/test-repo/pull-requests/1/testing-pr-flow`,
		},
	}

	answer, err := Event.Execute(msg)
	assert.NoError(t, err)

	expectedText := "I found the next pull-requests:\nPull-request #1 [repository: test-repo]\n\nThese pull-requests cannot be merged:\nhttps://bitbucket.org/john/test-repo/pull-requests/1 - The pull-request should be approved by one of the required reviewers. \n\nThere is no pull-requests, which can be merged.\nNothing to release"
	assert.Equal(t, expectedText, answer.Text)
}

func TestBitBucketReleaseEvent_Execute_ErrorDuringPRMerge(t *testing.T) {
	container.C.Config.BitBucketConfig.RequiredReviewers = []config.BitBucketReviewer{
		{
			UUID:     "{test-uid}",
			SlackUID: "TESTSLACKID",
		},
		{
			UUID:     "{test-second-uid}",
			SlackUID: "TESTSECONDSLACKID",
		},
	}

	//PullRequest status OPEN but no participants
	container.C.BibBucketClient = &mock.MockedBitBucketClient{
		IsTokenInvalid: true,
		PullRequestInfoResponse: dto.BitBucketPullRequestInfoResponse{
			Title:       "Some title",
			Description: "Feature;Some task description;\\(https://some-url.net/browse/error-502\\);JohnDoeProject",
			State:       pullRequestStateOpen,
			Participants: []dto.Participant{
				dto.Participant{
					User: dto.ParticipantUser{
						UUID: "{test-uid}",
					},
					Approved: true,
				},
			},
		},
		MergePullRequestError: errors.New("Failed to merge "),
	}

	var msg = dto.SlackRequestChatPostMessage{
		OriginalMessage: dto.SlackResponseEventMessage{
			Text: `release these pull-requests to production: https://bitbucket.org/john/test-repo/pull-requests/1/testing-pr-flow`,
		},
	}

	answer, err := Event.Execute(msg)
	assert.Error(t, err)

	expectedText := "I found the next pull-requests:\nPull-request #1 [repository: test-repo]\n\nAll pull-requests are ready for merge! This is awesome!\nNext pull-requests is will be merged:\n[#1] https://bitbucket.org/john/test-repo/pull-requests/1 \nWe have only one pull-request, so I will try to merge it directly to the main branch.\nI cannot merge the pull-request #1 because of error `Failed to merge `\n"
	assert.Equal(t, expectedText, answer.Text)
}

func TestBitBucketReleaseEvent_Execute_PRMerged(t *testing.T) {
	container.C.Config.BitBucketConfig.RequiredReviewers = []config.BitBucketReviewer{
		{
			UUID:     "{test-uid}",
			SlackUID: "TESTSLACKID",
		},
		{
			UUID:     "{test-second-uid}",
			SlackUID: "TESTSECONDSLACKID",
		},
	}

	//PullRequest status OPEN but no participants
	container.C.BibBucketClient = &mock.MockedBitBucketClient{
		IsTokenInvalid: true,
		PullRequestInfoResponse: dto.BitBucketPullRequestInfoResponse{
			Title:       "Some title",
			Description: "Feature;Some task description;\\(https://some-url.net/browse/error-502\\);JohnDoeProject",
			State:       pullRequestStateOpen,
			Participants: []dto.Participant{
				dto.Participant{
					User: dto.ParticipantUser{
						UUID: "{test-uid}",
					},
					Approved: true,
				},
			},
		},
		MergePullRequestResponse: dto.BitBucketPullRequestInfoResponse{
			Title:       "Some title",
			Description: "Feature;Some task description;\\(https://some-url.net/browse/error-502\\);JohnDoeProject",
			State:       pullRequestStateMerged,
			Participants: []dto.Participant{
				dto.Participant{
					User: dto.ParticipantUser{
						UUID: "{test-uid}",
					},
					Approved: true,
				},
			},
		},
	}

	var msg = dto.SlackRequestChatPostMessage{
		OriginalMessage: dto.SlackResponseEventMessage{
			Text: `release these pull-requests to production: https://bitbucket.org/john/test-repo/pull-requests/1/testing-pr-flow`,
		},
	}

	answer, err := Event.Execute(msg)
	assert.NoError(t, err)

	expectedText := "I found the next pull-requests:\nPull-request #1 [repository: test-repo]\n\nAll pull-requests are ready for merge! This is awesome!\nNext pull-requests is will be merged:\n[#1] https://bitbucket.org/john/test-repo/pull-requests/1 \n\nWe have only one pull-request, so I will try to merge it directly to the main branch.\n\nI merged pull-request #`1` into destination branch of repository `test-repo` :)\n"
	assert.Equal(t, expectedText, answer.Text)
}
