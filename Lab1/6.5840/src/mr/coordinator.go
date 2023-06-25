package mr

import (
	"log"
	"net"
	"net/http"
	"net/rpc"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// 任务
type Task struct {
	taskId   int
	taskType TaskTypes // 任务类型
	fileName []string  // 保存读取文件和写入文件名
}

type Coordinator struct {
	// Your definitions here.
	nReduce          int // 主函数传进来的
	nMapTask         int // 即主函数传入的文件数目
	nReduceTask      int
	chanMap          chan Task
	chanReduce       chan Task
	mapTaskState     map[int]bool // 标记所有 map 任务是否完成
	reduceTaskState  map[int]bool
	reduceFilePath   map[int][]string // 保存 map 任务生成的 file 路径，reduce 读取
	taskStateMutex   sync.Mutex
	filePathMutex    sync.Mutex
	isMapTaskDone    atomic.Bool
	isReduceTaskDone atomic.Bool
}

var nowMapWorkerId atomic.Int32
var nowReduceWorkerId atomic.Int32
var nowTaskId atomic.Int32

// Your code here -- RPC handlers for the worker to call.
// 改写 func (c *Coordinator) Example(args *ExampleArgs, reply *ExampleReply) error，处理请求
func (c *Coordinator) ProcessTask(args *TaskArgs, reply *TaskReply) error {
	switch args.MessageType {
	case TaskRequest:
		c.ProcessTaskRequest(args, reply)
	case TaskFinish:
		c.ProcessTaskFinish(args, reply)
	}
	return nil
}

// an example RPC handler.
// the RPC argument and reply types are defined in rpc.go.
func (c *Coordinator) Example(args *ExampleArgs, reply *ExampleReply) error {
	reply.Y = args.X + 1
	return nil
}

// 处理 rpc 中 worker 发来的请求任务信息，分发任务
func (c *Coordinator) ProcessTaskRequest(args *TaskArgs, reply *TaskReply) {
	if !c.isMapTaskDone.Load() {
		select {
		case task := <-c.chanMap:
			// fmt.Println("request map task", task)
			reply.TaskId = task.taskId
			reply.TaskType = MapTask
			reply.FileNameList = append(reply.FileNameList, task.fileName[0])
			reply.WorkerId = int(nowMapWorkerId.Load())
			reply.NReduce = c.nReduce
			nowMapWorkerId.Add(1)
			go c.checkTimeOut(&task)
		default:
			reply.TaskType = Wait
			reply.WorkerId = 1
		}
	} else if !c.isReduceTaskDone.Load() {
		select {
		case task := <-c.chanReduce:
			// fmt.Println("request reduce task", task)
			reply.TaskId = task.taskId
			reply.TaskType = ReduceTask
			reply.FileNameList = task.fileName
			reply.WorkerId = int(nowReduceWorkerId.Load())
			nowReduceWorkerId.Add(1)
			go c.checkTimeOut(&task)
		default:
			reply.TaskType = Wait
			reply.WorkerId = 1
		}
	}
	return
}

// 处理 rpc 中 worker 发来的请求任务信息，分发任务
func (c *Coordinator) ProcessTaskFinish(args *TaskArgs, reply *TaskReply) {
	switch args.TaskType {
	case MapTask:
		// fmt.Println("map task finished, ", args)
		c.filePathMutex.Lock()
		addReduceFilePath(args.FilePath, c)
		c.filePathMutex.Unlock()

		flag := false
		c.taskStateMutex.Lock()
		if !c.mapTaskState[args.TaskId] {
			c.nMapTask--
			if c.nMapTask <= 0 {
				c.sendTaskToReduce()
				flag = true
			}
		}
		c.mapTaskState[args.TaskId] = true
		c.taskStateMutex.Unlock()
		c.isMapTaskDone.Store(flag)
	case ReduceTask:
		// fmt.Println("reduce task finished, ", args)
		c.taskStateMutex.Lock()
		flag := false
		if !c.reduceTaskState[args.TaskId] {
			c.nReduceTask--
			if c.nReduceTask <= 0 {
				flag = true
			}
		}
		c.reduceTaskState[args.TaskId] = true
		c.taskStateMutex.Unlock()
		c.isReduceTaskDone.Store(flag)
	}
}

// 每次将任务分发出去后，创建一个协程等待10s，若10s后该任务仍未完成则认为处理该任务的 worker 出故障了
// 将任务重新写回管道
func (c *Coordinator) checkTimeOut(task *Task) {
	time.Sleep(10 * time.Second)
	c.taskStateMutex.Lock()
	defer c.taskStateMutex.Unlock()
	switch task.taskType {
	case MapTask:
		if !c.mapTaskState[task.taskId] {
			c.chanMap <- *task
		}
	case ReduceTask:
		if !c.reduceTaskState[task.taskId] {
			c.chanReduce <- *task
		}
	}
}

// 将 map 完成的作业按照 reduce 的编号增加到相应的文件列表中
func addReduceFilePath(files []string, c *Coordinator) {
	for _, filePath := range files {
		// fmt.Println("filePath: ", filePath)
		index := strings.LastIndex(filePath, "-")
		reduceId, _ := strconv.Atoi(filePath[index+1:])
		c.reduceFilePath[reduceId] = append(c.reduceFilePath[reduceId], filePath)
	}
}

// 将任务分给 map 处理
func (c *Coordinator) sendTaskToMap(files []string) {
	for i, file := range files {
		task := Task{
			taskId:   i,
			taskType: MapTask,
			fileName: []string{file},
		}
		c.chanMap <- task
	}
}

// 将作业分给 reduce 处理
func (c *Coordinator) sendTaskToReduce() {
	nReduce := int(c.nReduce)
	for i := 0; i < nReduce; i++ {
		task := Task{
			taskId:   i,
			taskType: ReduceTask,
			fileName: c.reduceFilePath[i],
		}
		c.chanReduce <- task
	}
}

// start a thread that listens for RPCs from worker.go
func (c *Coordinator) server() {
	rpc.Register(c)
	rpc.HandleHTTP()
	//l, e := net.Listen("tcp", ":1234")
	sockname := coordinatorSock()
	os.Remove(sockname)
	l, e := net.Listen("unix", sockname)
	if e != nil {
		log.Fatal("listen error:", e)
	}
	go http.Serve(l, nil)
}

// main/mrcoordinator.go calls Done() periodically to find out
// if the entire job has finished.
func (c *Coordinator) Done() bool {
	// Your code here.
	return c.isReduceTaskDone.Load()
}

// create a Coordinator.
// main/mrcoordinator.go calls this function.
// nReduce is the number of reduce tasks to use.
// 初始化 Coordinator
func MakeCoordinator(files []string, nReduce int) *Coordinator {
	// fmt.Println("Coordinator init")
	c := Coordinator{}
	// 初始化
	c.nReduce = nReduce
	c.nMapTask = len(files)
	c.nReduceTask = c.nReduce
	c.chanMap = make(chan Task, c.nMapTask)
	c.chanReduce = make(chan Task, c.nReduce)
	c.mapTaskState = make(map[int]bool)
	c.reduceTaskState = make(map[int]bool)
	c.reduceFilePath = make(map[int][]string)
	c.isMapTaskDone.Store(false)
	c.isReduceTaskDone.Store(false)
	nowMapWorkerId.Store(1000)
	nowReduceWorkerId.Store(100)
	nowTaskId.Store(0)
	// 将所有文件分给 map 开始处理
	go c.sendTaskToMap(files)
	c.server()
	return &c
}
