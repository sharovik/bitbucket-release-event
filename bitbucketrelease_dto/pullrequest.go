package bitbucketrelease_dto

//PullRequest the pull-request item
type PullRequest struct {
	ID             int64
	RepositorySlug string
	BranchName     string
	Workspace      string
	Title          string
	Description    string
}
