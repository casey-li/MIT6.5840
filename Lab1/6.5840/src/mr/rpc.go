package mr

//
// RPC definitions.
//
// remember to capitalize all names.
//

import (
	"os"
	"strconv"
)

//
// example to show how to declare the arguments
// and reply for an RPC.
//

type ExampleArgs struct {
	X int
}

type ExampleReply struct {
	Y int
}

// Add your RPC definitions here.

type TaskArgs struct {
	TaskId      int
	TaskType    TaskTypes
	MessageType MessageTypes
	FilePath    []string
}

type TaskReply struct {
	TaskId       int
	TaskType     TaskTypes
	FileNameList []string
	WorkerId     int
	NReduce      int
}

// 任务类别
type TaskTypes int

const (
	MapTask TaskTypes = iota + 1
	ReduceTask
	Wait
)

// 信息类别是 请求任务 还是 已完成任务
type MessageTypes int

const (
	TaskRequest MessageTypes = iota + 1
	TaskFinish
)

// Cook up a unique-ish UNIX-domain socket name
// in /var/tmp, for the coordinator.
// Can't use the current directory since
// Athena AFS doesn't support UNIX-domain sockets.
func coordinatorSock() string {
	s := "/var/tmp/5840-mr-"
	s += strconv.Itoa(os.Getuid())
	return s
}
