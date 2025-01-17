package parallel

import (
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/lindb/lindb/models"
	"github.com/lindb/lindb/rpc"
	pb "github.com/lindb/lindb/rpc/proto/common"
)

func TestTaskManager_ClientStream(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	currentNode := models.Node{IP: "1.1.1.1", Port: 8000}
	taskClientFactory := rpc.NewMockTaskClientFactory(ctrl)
	taskServerFactory := rpc.NewMockTaskServerFactory(ctrl)

	taskManager1 := NewTaskManager(currentNode, taskClientFactory, taskServerFactory)

	taskCtx := newTaskContext("xxx", IntermediateTask, "parentTaskID", "parentNode", 2)
	taskManager1.Submit(taskCtx)

	assert.Equal(t, taskCtx, taskManager1.Get("xxx"))
	assert.Nil(t, taskManager1.Get("xxx11"))

	taskManager2 := taskManager1.(*taskManager)
	taskManager2.tasks.Store("xxx11", nil)
	assert.Nil(t, taskManager1.Get("xxx11"))

	taskCtx = newTaskContext("taskID", IntermediateTask, "parentTaskID", "parentNode", 2)
	taskManager1.Submit(taskCtx)
	assert.Equal(t, taskCtx, taskManager1.Get("taskID"))
	taskManager1.Complete("taskID")
	assert.Nil(t, taskManager1.Get("taskID"))

	assert.Equal(t, "1.1.1.1:8000-1", taskManager1.AllocTaskID())
	assert.Equal(t, "1.1.1.1:8000-2", taskManager1.AllocTaskID())
	assert.Equal(t, "1.1.1.1:8000-3", taskManager1.AllocTaskID())
}

func TestTaskManager_SendRequest(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	currentNode := models.Node{IP: "1.1.1.1", Port: 8000}
	taskClientFactory := rpc.NewMockTaskClientFactory(ctrl)

	taskManager := NewTaskManager(currentNode, taskClientFactory, nil)
	taskClientFactory.EXPECT().GetTaskClient("targetNode").Return(nil)
	err := taskManager.SendRequest("targetNode", nil)
	assert.NotNil(t, err)

	client := pb.NewMockTaskService_HandleClient(ctrl)
	taskClientFactory.EXPECT().GetTaskClient("targetNode").Return(client).MaxTimes(2)
	client.EXPECT().Send(gomock.Any()).Return(fmt.Errorf("err"))
	err = taskManager.SendRequest("targetNode", nil)
	assert.NotNil(t, err)

	client.EXPECT().Send(gomock.Any()).Return(nil)
	err = taskManager.SendRequest("targetNode", nil)
	if err != nil {
		t.Fatal(err)
	}
}

func TestTaskManager_SendResponse(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	currentNode := models.Node{IP: "1.1.1.1", Port: 8000}
	taskServerFactory := rpc.NewMockTaskServerFactory(ctrl)

	taskManager := NewTaskManager(currentNode, nil, taskServerFactory)
	taskServerFactory.EXPECT().GetStream("targetNode").Return(nil)
	err := taskManager.SendResponse("targetNode", nil)
	assert.NotNil(t, err)

	server := pb.NewMockTaskService_HandleServer(ctrl)
	taskServerFactory.EXPECT().GetStream("targetNode").Return(server).MaxTimes(2)
	server.EXPECT().Send(gomock.Any()).Return(fmt.Errorf("err"))
	err = taskManager.SendResponse("targetNode", nil)
	assert.NotNil(t, err)

	server.EXPECT().Send(gomock.Any()).Return(nil)
	err = taskManager.SendResponse("targetNode", nil)
	if err != nil {
		t.Fatal(err)
	}
}
