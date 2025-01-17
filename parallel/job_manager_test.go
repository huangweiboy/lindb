package parallel

import (
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/lindb/lindb/models"
)

func TestJobManager_SubmitJob(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	taskManager := NewMockTaskManager(ctrl)
	taskManager.EXPECT().Submit(gomock.Any()).AnyTimes()
	taskManager.EXPECT().AllocTaskID().Return("TaskID").AnyTimes()

	jobManager := NewJobManager(taskManager)
	physicalPlan := models.NewPhysicalPlan(models.Root{Indicator: "1.1.1.3:8000", NumOfTask: 1})
	physicalPlan.AddLeaf(models.Leaf{
		BaseNode: models.BaseNode{
			Parent:    "1.1.1.3:8000",
			Indicator: "1.1.1.1:9000",
		},
		ShardIDs: []int32{1, 2, 4},
	})
	taskManager.EXPECT().SendRequest(gomock.Any(), gomock.Any()).Return(fmt.Errorf("err"))
	err := jobManager.SubmitJob(NewJobContext(nil, physicalPlan))
	assert.NotNil(t, err)

	taskManager.EXPECT().SendRequest(gomock.Any(), gomock.Any()).Return(nil)
	err = jobManager.SubmitJob(NewJobContext(nil, physicalPlan))
	if err != nil {
		t.Fatal(err)
	}
}

func TestJobManager_SubmitJob_2(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	taskManager := NewMockTaskManager(ctrl)
	taskManager.EXPECT().Submit(gomock.Any()).AnyTimes()
	taskManager.EXPECT().AllocTaskID().Return("TaskID").AnyTimes()

	jobManager := NewJobManager(taskManager)
	physicalPlan := models.NewPhysicalPlan(models.Root{Indicator: "1.1.1.3:8000", NumOfTask: 1})
	physicalPlan.AddIntermediate(models.Intermediate{
		BaseNode: models.BaseNode{
			Parent:    "1.1.1.3:8000",
			Indicator: "1.1.1.1:9000",
		},
	})

	taskManager.EXPECT().SendRequest(gomock.Any(), gomock.Any()).Return(fmt.Errorf("err"))
	err := jobManager.SubmitJob(NewJobContext(nil, physicalPlan))
	assert.NotNil(t, err)

	taskManager.EXPECT().SendRequest(gomock.Any(), gomock.Any()).Return(nil)
	err = jobManager.SubmitJob(NewJobContext(nil, physicalPlan))
	if err != nil {
		t.Fatal(err)
	}
	assert.NotNil(t, jobManager.GetTaskManager())
}

func TestJobManager_GetTaskManager(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	taskManager := NewMockTaskManager(ctrl)
	jobManager1 := NewJobManager(taskManager)
	manager := jobManager1.(*jobManager)
	manager.jobs.Store(int64(1), &jobContext{})
	job := jobManager1.GetJob(1)
	assert.NotNil(t, job)
	job = jobManager1.GetJob(2)
	assert.Nil(t, job)
	manager.jobs.Store(int64(2), "test")
	job = jobManager1.GetJob(2)
	assert.Nil(t, job)
}
