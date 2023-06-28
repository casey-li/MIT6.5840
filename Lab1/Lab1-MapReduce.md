# :two_hearts: Lab 1、MapReduce

[实验介绍](https://pdos.csail.mit.edu/6.824/labs/lab-mr.html)

## :cherry_blossom: 目标
实现一个简易版的分布式 mapreduce demo, 并通过 `test-mr.sh` 中的所有测试即可

- :one: word count test
- :two: indexer test
- :three: map parallelism test
- :four: reduce parallelism test
- :five: job count test
- :six: early exit test
- :seven: crash test

## :mag: 提示

- :one: 可以借鉴 `mrsequential.go` 中的内容, 知道应该干什么
- :two: 中间文件可以命名为 mr-X-Y 的形式，其中 X 为 Map 任务编号，Y 为 reduce 任务编号; 结果文件命名为 mr-output-Y
- :three: 可以利用 `encoding/json` 包来对生成的键值对进行管理
- :four: `ihash(key)` 函数用于将给定的键映射到相应的 Reduce 任务中，即落到相应的中间文件
- :five: worker 有时是需要阻塞等待的，因为必须最后一个 map 任务完成后 reduce 任务才能启动, 所以 coordinator 可以给 worker 下发等待的命令
- :six: 可以给所有任务打上时间戳来判断 worker 是否崩溃, 并基于此重新分发当前任务
- :seven: worker 在崩溃时可能会遇到文件已经部分写入的情况，可以使用ioutil.TempFile创建临时文件，然后用 os.Rename 进行原子命名
- :eight: 记得加锁

**论文中最重要的架构图**

![Fig. 1](https://github.com/SwordHarry/MIT6.824_2021_note/raw/main/lab/img/008i3skNgy1gskdg2ig4tj30zq0okwgf.png)


## :pizza: 数据结构

`rpc.go` 中 需要改写提供的两个结构体，分别表示 参数 以及 回复

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

`worker.go` 中已经提供了键值对的结构体 KeyVaule，以及相应的哈希函数

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

`coordinator.go` 中定义任务以及协调器两个结构体

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

## :beers: 主要函数

只需实现 mr 目录下的 `worker.go` 和 `coordinator.go`，由 main 目录下的 `mrcoordinator.go` 和 `mrworker.go` 进行调用

- `mrcoordinator.go` 调用 `mr.MakeCoordinator(os.Args[1:], 10)`
- `mrworker.go` 调用 `mr.Worker(mapf, reducef)`

### :cherry_blossom: `worker.go` 部分

**调用函数**

```go
// 由 main/mrworker.go 调用，传入 map 和 reduce 函数
func Worker(mapf func(string, string) []KeyValue, reducef func(string, []string) string)
```

**`worker.go` 的流程**

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

### :cherry_blossom: `coordinator.go` 部分

**调用函数**
```go
// 由 main/mrcoordinator.go 调用，传入 map 的文件列表 (即 MapTask 的数目) 和 ReduceTask 的数目
func MakeCoordinator(files []string, nReduce int) *Coordinator
```

**`coordinator.go` 的流程**
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

