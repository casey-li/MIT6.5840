# Lab1、MapReduce

只需实现 mr 目录下的 `worker.go` 和 `coordinator.go`，由 main 目录下的 `mrcoordinator.go` 和 `mrworker.go` 进行调用

- `mrcoordinator.go` 调用 `mr.MakeCoordinator(os.Args[1:], 10)`
- `mrworker.go` 调用 `mr.Worker(mapf, reducef)`

## 一、数据结构

:tulip: `rpc.go` 中 需要改写提供的两个结构体，分别表示 参数 以及 回复

```go
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
```

:tulip: `worker.go` 中已经提供了键值对的结构体 KeyVaule，以及相应的哈希函数

```go
type ByKey []KeyValue

// for sorting by key.
func (a ByKey) Len() int           { return len(a) }
func (a ByKey) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByKey) Less(i, j int) bool { return a[i].Key < a[j].Key }

// Map functions return a slice of KeyValue.
type KeyValue struct {
	Key   string
	Value string
}

// use ihash(key) % NReduce to choose the reduce
// task number for each KeyValue emitted by Map.
func ihash(key string) int {
	h := fnv.New32a()
	h.Write([]byte(key))
	return int(h.Sum32() & 0x7fffffff)
}
```

:tulip: `coordinator.go` 中定义任务以及协调器两个结构体

```go
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
```

## 二、主要流程


### `worker.go` 部分

:cherry_blossom: **调用函数**

```go
// 由 main/mrworker.go 调用，传入 map 和 reduce 函数
func Worker(mapf func(string, string) []KeyValue, reducef func(string, []string) string)
```

:cherry_blossom: **`worker.go` 的流程**

1. 发起 RPC 调用, 设置 TaskArgs 的 MessageType 为 TaskRequest, 调用 coordinator 的 ProcessTask, 等待响应 TaskReply
2. 检查 TaskReply 的 TaskType 
    - 为 MapTask 
        1. 读取输入文件
        2. 调用 mapf 函数，生成 KeyValue 切片
		3. 生成 nReduce 个临时文件，返回对应的 `*json.Encoder` 对象的切片
        4. 遍历 KeyValue 切片，根据键的哈希值计算出对应的编码器,，对键值对进行编码，并写入临时文件
        5. 删除临时文件
        6. 发起 RPC 调用, 设置 TaskArgs 的 MessageType 为 TaskFinish, 调用 coordinator 的 ProcessTask
    - 为 ReduceTask
        1. 读取多个中间文件的信息并排序
        2. 依次提取出同一个 key 的所有value
        3. 调用 reducef 函数得到处理的结果
        4. 保存结果
        5. 发起 RPC 调用, 设置 TaskArgs 的 MessageType 为 TaskFinish, 调用 coordinator 的 ProcessTask

### `coordinator.go` 部分

:cherry_blossom: **调用函数**
```go
// 由 main/mrcoordinator.go 调用，传入 map 的文件列表 (即 MapTask 的数目) 和 ReduceTask 的数目
func MakeCoordinator(files []string, nReduce int) *Coordinator
```

:cherry_blossom: **`coordinator.go` 的流程**
1. 初始化 coordinator, 并将所有 MapTask 写入管道
2. 启动服务，监听来自 `worker.go` 发来的 RPC 请求
3. 检查发来的请求 (TaskArgs) 中的消息类别 (MessageTypes)
    - 为请求任务 (TaskRequest), 检查当前 coordinator 的状态
        1. 若所有 MapTask 未完成，则从 chanMap 中读取一个任务，否则从 chanReduce 中读取一个任务
        2. 设置 TaskReply 的参数，启动一个协程对当前任务进行超时检测
        3. 协程休息 10s 后检测当前任务的完成情况，若仍未完成则将任务重新写回管道
    - 为完成了任务 (TaskFinish), 检查 TaskArgs 的 TaskType
        - 为 MapTask
            1. 将生成的文件分类处理，保存到每个 ReduceTask 对应的文件列表中
            2. 检查并修改当前 MapTask 对应的 mapTaskState 以及 nMapTask
            3. 若 nMapTask <= 0, 修改 coordinator 的标志位 (isMapTaskDone) 并将所有 ReduceTask 写入管道
        - 为 ReduceTask
            1. 检查并修改当前 ReduceTask 对应的 reduceTaskState 以及 nReduceTask
            2. 若 nReduceTask <= 0, 修改 coordinator 的标志位 (isReduceTaskDone) ,主函数检测到已完成后结束

