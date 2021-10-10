package bitbucketrelease

import (
	"github.com/sharovik/devbot/internal/container"
	"github.com/sharovik/orm/clients"
	"github.com/sharovik/orm/dto"
	"github.com/sharovik/orm/query"
)

type UpdateReleaseTriggerMigration struct {
}

func (m UpdateReleaseTriggerMigration) GetName() string {
	return "update-trigger-message"
}

func (m UpdateReleaseTriggerMigration) Execute() error {
	eventID, err := container.C.Dictionary.FindEventByAlias(EventName)
	if err != nil {
		return err
	}

	scenarioID, err := container.C.Dictionary.InsertScenario("Bitbucket release scenario", eventID)
	if err != nil {
		return err
	}

	_, err = container.C.Dictionary.InsertQuestion("release", "Give me a second", scenarioID, "(?im)(release)", "")
	if err != nil {
		return err
	}

	if err := updateCurrentQuestion(); err != nil {
		return err
	}

	return nil
}

func updateCurrentQuestion() error {
	var model = dto.BaseModel{
		TableName: "questions",
		Fields: []interface{}{
			dto.ModelField{
				Name:    "answer",
				Value: "Alright. But next time, please use the `release` instead of `bb release`. I understand now both. Just in case ;)",
			},
		},
	}
	_, err := container.C.Dictionary.GetNewClient().Execute(new(clients.Query).Update(&model).Where(query.Where{
		First:    "question",
		Operator: "LIKE",
		Second:   `"%bb release%"`,
	}))
	if err != nil {
		return err
	}

	return nil
}
