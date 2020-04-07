# BitBucket Release event
This event is a part of the [devbot project](https://github.com/sharovik/devbot) which can be used for automation of your daily development routine.

You can use this event for release optimisation of your project/projects. In the message text be specified 1 or multiple pull-requests for multiple repositories, if they are good for merge, then event will try to merge all of them into main branch of repository.
The event accepts multiple pull-requests for multiple repositories. If there is more than one pull-request per repository, then event will create a **release pull-request**, which should be approved by on of required reviewers and will send this pull-request link in the answer of the message.

## Table of contents
- [Prerequisites](#prerequisites)
- [How it works](#how-it-works)

## Prerequisites
Before you will start use this event please be aware of these steps

### Clone into devbot project
```
git clone git@github.com:sharovik/bitbucket-release-event.git events/bitbucket_release
```

### Install it into your devbot project
1. clone this repository into `events/` folder of your devbot project. Please make sure to use `bitbucket_release` folder name for this event 
2. add into imports path to this event in `defined-events.go` file
``` 
import "github.com/sharovik/devbot/events/bitbucket_release"
```
3. add this event into `defined-events.go` file to the defined events map object
``` 
DefinedEvents.Events[bitbucket_release.EventName] = bitbucket_release.Event
```

### Prepare environment variables in your .env
Copy and paste everything from the **#Bitbucket** section in `.env.example` file into `.env` file

### Create BitBucket client
Here [you can find how to do it](https://github.com/sharovik/devbot/blob/master/documentation/bitbucket_client_configuration.md).

## How it works
You send the message to the PM of bot with the next text: 
```
bb release
https://bitbucket.org/{your-workspace}/{your-first-repository}/pull-requests/1/readmemd-edited-online-with-bitbucket/diff
https://bitbucket.org/{your-workspace}/{your-second-repository}/pull-requests/20
https://bitbucket.org/{your-workspace}/{your-second-repository}/pull-requests/36/release-pull-request/diff
https://bitbucket.org/your-workspace}/{your-first-repository/pull-requests/35/release-pull-request/diff
```
In the answer you will receive the status update of the merge process.

### The main diagram
Here you can see the main flow how this event works

![The main flow](documentation/images/bitbucket-release-event.png)

### The pull-request checks
In this diagram you can see how the current pull-request check works

![The pull-request check](documentation/images/the-pull-request-check.png)

### The release process
In this diagram you can see the flow of the release

![The flow of the release](documentation/images/release-process.png)
