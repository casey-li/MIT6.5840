# :two_hearts: Lab 3、Fault-tolerant Key/Value Service

[实验介绍](https://pdos.csail.mit.edu/6.824/labs/lab-kvraft.html)

本实验将使用实验 2 中的 Raft 库构建容错键/值存储服务，即维护一个简单的键/值对数据库，其中键和值都是字符串。具体来说，该服务是一个复制状态机，由多个使用 Raft 进行复制的键/值服务器组成，只要大多数服务器处于活动状态并且可以通信，该服务就应该继续处理客户端请求

实验 3 之后将实现 Raft 交互图中的所有部分

![Raft交互图](https://github.com/casey-li/MIT6.5840/blob/main/Lab3/Lab3A/result/pic/Raft%E5%9B%BE.jpg?raw=true)

### :four_leaf_clover: 客户端以及服务器的职责

**客户端 (Clerk)**

客户端可以向键/值服务发送三种不同的 `RPC (Put, Append, Get)`
- :one: `Put(key, value)` 替换数据库中特定键的值
- :two: `Append(key, arg)` 将 `arg` 附加到键的值
- :three: `Get(key)` 获取键的当前值

对不存在的键的 `Get` 应返回空字符串, 追加到不存在的键应该像 `Put` 一样。每个客户端都通过 Clerk 使用 `Put / Append / Get` 方法与服务进行通信，Clerk 管理与服务器的 `RPC` 交互

**服务器 (KVServer)**

服务器必须实现对 Clerk `Get / Put / Append` 方法调用的**线性一致性**

**线性一致性**的难点在于所有服务器必须为并发请求选择相同的执行顺序，必须避免使用不是最新的状态回复客户端，并且必须持久化处理所有已知的客户端的信息以便在发生故障后恢复其状态

### **:mag: 提示** 

Lab 3 共有两个实验：
- :one: Lab 3A 使用 Raft 实现来实现键/值服务，但不使用快照
- :two: Lab 3B 将使用 Lab 2D 中的快照，即允许 Raft 丢弃旧的日志条目

`src/kvraft` 中提供了代码框架和测试案例。仅需要修改 `kvraft/client.go`, `kvraft/server.go` 和 `kvraft/common.go`

---

## :wink: Lab 3A - Key/value service without snapshots

### :cherry_blossom: 目标

使用 Lab 2 的 Raft 库构建键值服务器 KVServer ，构建客户端 Clerk。构建的 KVServer 需支持 `Put, Get, Append` 操作，并且所有操作需满足线性一致性

### :maple_leaf: 效果

在网络和服务器出现故障的情况下，KVServer 仍能继续运行。此外，必须确保 Clerk 重新发送请求不会导致服务器多次执行同一个请求（包括 Clerk 在一个任期内向 kvserver 领导者发送请求、等待答复超时并在另一个任期内将请求重新发送给新领导者的情况。该请求应该只执行一次）

### :mag: 提示

客户端给非 leader 节点发送 RPC 或者超时了仍未收到回复的话，Clerk 换一个 KVServer 继续尝试。若操作未能提交，如 leader 被替换了的话，KVServer 应该报告错误，然后 Clerk 使用另一个服务器重试

kvservers 之间不应该直接通信，只能通过 Raft 进行交流！

- :one: 调用 `Start()` 后，KVServer 将需要等待 Raft 完成协议。已达成一致的命令会被发送到 `applyCh`，当 `PutAppend()` 和 `Get()` 处理程序使用 `Start()` 将命令提交到 Raft 日志后，需要继续读取 applyCh。请注意 kvserver 及其 Raft 库之间的死锁
- :two: 可以向 Raft `ApplyMsg` 添加字段，也可以向 Raft RPC 添加字段 ( 如 `AppendEntries`)，但这对于大多数实现来说不是必需的
- :three: 若 KVServer 不属于多数服务器所在的集群，就不应该相应 `Get() RPC`以避免提供陈旧数据。一个简单的解决方案是将 `Get()` 同 `Put(), Append()` 一样添加到 Raft 日志中，若能达成共识就响应
- :four: 建议从一开始就加锁，因为设计避免死锁的代码会影响整体代码设计。使用 `go test -race` 检查您的代码是否有竞争
- :five: 需要处理领导者改变的情况（如虽然一个领导者已经调用了 `Start()`，但在请求提交到日志之前失去了领导地位，此时应该让 Clerk 重新发送请求到其他服务器，直到找到新的领导者）。为了实现此目的，可以检查 Start() 返回的索引处的日志是否发生了变化，也可以检查 Raft 的任期是否更改。如果前领导者自己分区，它不会知道新领导者；但是同一分区中的任何客户端也无法与新的领导者通信，因此在这种情况下，服务器和客户端可以无限期地等待，直到分区修复
- :six: 可能需要修改您的 Clerk 以记住哪台服务器是上一次 RPC 的领导者，并首先将下一次 RPC 发送到该服务器。这将避免浪费时间在每个 RPC 上搜索领导者，这可能会帮助您足够快地通过一些测试
- :seven: 注意需要唯一地标识客户端操作，以确保 KVServer 仅执行每个操作一次
- :eight: 设计的命令重复检测方案应能快速释放服务器内存

基本思路以及一些难以考虑到的特殊情况都在提示中给出来了，所以 Lab3A 的难度不算很大


### :dart: 思路

相比于 Lab 2, Lab 3 涉及到了客户端, 服务端以及底层的 Raft, 不在实现前理清它们之间的关系的话很容易混乱, 为了方便理清思路, 自己画了下面的正常情况下的总体流程图

![正常情况下的总体流程图](https://github.com/casey-li/MIT6.5840/blob/main/Lab3/Lab3A/result/pic/3A%E6%80%BB%E4%BD%93%E7%A4%BA%E6%84%8F%E5%9B%BE.png?raw=true)

- :one: Client 给 KVServer(底层 Raft 为 Leader) 发送 `RPC request`
- :two: KVServer 封装 Op 并调用 Raft 的 `Start()` 提交日志
- :three: 底层 Raft 达成共识
- :four: apply 该命令让 KVServer 进行处理 
- :five: KVServer 返回 `RPC response`

上图为正常情况下的总体流程图, 具体到 leader 和 follower Raft 对应的 KVServer 仍有些许差异 (Leader 对应的 KVServer 不仅需要执行命令, 还需要在一定时间内生成 response), 一次具体的请求流程 (包含超时检测) 如下

![一次具体的请求流程](https://github.com/casey-li/MIT6.5840/blob/main/Lab3/Lab3A/result/pic/3A%E5%8D%95%E6%AC%A1%E8%AF%B7%E6%B1%82%E7%A4%BA%E6%84%8F%E5%9B%BE.png?raw=true)

***Leader KVServer***

- :one: Client 给 KVServer 的 Leader 发送 `RPC request`
- :two: KVServer 封装 Op 并调用 Raft 的 `Start()` 提交日志, 因为需要在规定时间内给 Client 响应, 会同步开启定时器; 若未能在规定时间内在 `waitCh` 上读取到到相应的结果, 直接返回 `Err`
- :three: 底层 Raft 达成共识
- :four: Raft `apply` 该命令, 将其发送到 `applyCh`, 此时 KVServer 监听 `applyCh` 的协程会收到 `applyMsg` 
- :five: KVServer 根据 `applyMsg` 中的 Op 执行相应的命令, 给 `waitCh` 发送执行结果。这样正在处理 `RPC request` 的协程 (正在监听 `waitCh` 和定时器) 就会收到信息, 当然前提是此时未达到定时时间
- :six: KVServer 根据 `waitCh` 读取的结果生成 `RPC response`, 响应 Client

总的来说, KVServer 共在定时器超时或者收到 waitCh 的结果后都会给 Client 返回 RPC response, 注意这个流程中即使超时了命令也一定会执行!! 

因为只要是达成共识的命令必须执行 (当然需要判断是否为重复命令或过期的 RPC)。客户端收到超时回复后会另寻 KVServer, 但是其它 KVServer 不是 Leader, 所以会直接返回, 很快就又轮到这个 Leader KVServer
- 若当前命令是 Put / Append, 首先会检查是否为重复命令或者是否为过期的 RPC request, 发现已经执行过或为过期的 RPC response 都会直接返回 OK
- 若当前命令是 Get, 必定会再次提交给 Raft (需要确保自己不是小的网络分区中的旧 Leader), 继续上述流程 :three: - :six:

***Follower KVServer***

- :one: 等待达成共识。根据 Lab 2 实现的 Raft 可知会慢于 Leader 一拍 (达成共识后, Leader 提交命令, 更新 commitIndex; Follower 会在下一次收到心跳时发现自己的 commitIndex 落后于 Leader 的, 进而提交命令)
- :two: Raft `apply` 该命令, 将其发送到 `applyCh`, 此时 KVServer 监听 `applyCh` 的协程会收到 `applyMsg` 
- :three: KVServer 根据 `applyMsg` 中的 Op 执行相应的命令, 因为它无需给 Client 响应, 执行完后结束即可

***总结***

- Leader KVServer 流程: `Start(Op), create waitCh, create Timer (KVServer) -> wait until reach agreement (Rafts) -> apply() (Raft) -> receive applyMsg (KVServer) -> execute Op (KVServer) -> notify waitCh (KVServer) -> set reply and make response (KVServer)`
- Follower KVServer 流程: `receive Log (Raft) -> make AppendEntries response (Raft) -> wait next AppendEntries request (Raft) -> apply() (Raft) -> receive applyMsg (KVServer) -> execute Op (KVServer)`

### :pizza: 数据结构

#### :one: 请求和响应 RPC

代码框架中将 Put 和 Append 整合成了一种，因此共有两种 RPC 请求响应，自己在请求参数种新增了 RequestId 和 ClientId。服务端通过记录每个 ClientId 最近发来的 RequestId (执行过的命令)，就可以达到检验是否为过期 RPC 以及是否为重复命令的目的，因为 Clerk 每成功发送并收到响应后都会增加 RequestId

```go
const (
	OK             = "OK"
	ErrNoKey       = "ErrNoKey"
	ErrWrongLeader = "ErrWrongLeader"
)

type Err string

// Put or Append
type PutAppendArgs struct {
	Key   string
	Value string
	Op    string // "Put" or "Append"
    // You'll have to add definitions here.
	RequestId int64
	ClientId  int64
}

type PutAppendReply struct {
	Err Err
}

type GetArgs struct {
	Key string
    // You'll have to add definitions here.
	RequestId int64
	ClientId  int64
}

type GetReply struct {
	Err   Err
	Value string
}
```

#### :two: Clerk, KVServer 和 Op

- **Clerk**

每个 Clerk 需要有自己的身份标识，当前请求的id，所以增加了字段 `ClientId` 和 `RequestId`。因为要求中规定了它应该从上一次给它响应的 leader 服务器处开始尝试发送请求，所以增加了 `LeaderId`。Clerk 的完整结构体如下

```go
type Clerk struct {
	servers []*labrpc.ClientEnd
	// You will have to modify this struct.
	ClientId  int64
	LeaderId  int
	RequestId int64
}
```

- **KVServer**

KVServer 为一个键值数据库（键和值都为 string）, 因此首先必须要有一个 map 记录键值之间的映射关系 (`KVDB`)。为了达到去重以及检测过期 RPC 的要求, 记录了每个 `ClientId` 最近发来的 `RequestId` (执行过的命令), 即 `lastRequestMap`。因为 leader 需要给 Clerk 回复执行结果, 因此在底层 Raft 达成共识提交命令到 `applyCh` 并执行后, 当前 KVServer 需要知道执行结果。这里利用通道进行结果的传递, 因此增加了 `waitChMap` 字段 (key 为日志在 Raft 日志中的下标, value 为 Op 的通道)。KVServer的完整结构体如下

```go
type KVServer struct {
	mu      sync.Mutex
	me      int
	rf      *raft.Raft
	applyCh chan raft.ApplyMsg
	dead    int32 // set by Kill()

	maxraftstate int // snapshot if log grows this big

	// Your definitions here.
	KVDB           map[string]string // 状态机，记录KV
	waitChMap      map[int]chan *Op  // 通知 chan, key 为日志的下标，值为通道
	lastRequestMap map[int64]int64   // 保存每个客户端对应的最近一次请求的内容（包括请求的Id 和 回复）
}
```

- **Op**

操作结构体, KVServer 收到 Clerk 的 RPC 调用后，首先会封装一个 Op, 然后通过调用 `Start()` 将其提交到底层的 Raft 日志中。Op 的结构体需包含能唯一识别请求的字段 `(ClientId + RequestId)`, 操作类别 `(OpType)`, 键和值 `(Key + Value)`

```go
type Op struct {
	ClientId  int64
	RequestId int64 
	OpType    string
	Key       string
	Value     string
}
```

### :beers: 实现

#### :cherries: Get / PutAppend RPC

- :one: 客户端

客户端通过 RPC 调用服务器对应的函数; 根据提示, 客户端会根据收到的回复 (`Err`, 包括 `OK, ErrNoKey 和 ErrWrongLeader`) 不断进行尝试直到找到 leader 并收到正常的回复 (`OK 或 ErrNoKey`)

```go
func (ck *Clerk) Get(key string) string {
    // 设置 GetArgs, 不断尝试
	for {
		if ck.servers[ck.LeaderId].Call("KVServer.Get", &args, &reply) {
			switch reply.Err {
			case ErrWrongLeader:
				ck.LeaderId = (ck.LeaderId + 1) % len(ck.servers)
			case ErrNoKey, OK:
				ck.RequestId += 1
				return reply.Value
			}
		} else {
			ck.LeaderId = (ck.LeaderId + 1) % len(ck.servers)
		}
	}
}

func (ck *Clerk) PutAppend(key string, value string, op string) {
	// 设置 PutAppendArgs, 不断尝试
	for {
		if ck.servers[ck.LeaderId].Call("KVServer.PutAppend", &args, &reply) {
			switch reply.Err {
			case ErrWrongLeader:
				ck.LeaderId = (ck.LeaderId + 1) % len(ck.servers)
			case ErrNoKey, OK:
				ck.RequestId += 1
				return
			}
		} else {
			ck.LeaderId = (ck.LeaderId + 1) % len(ck.servers)
		}
	}
}

func (ck *Clerk) Put(key string, value string) {
	ck.PutAppend(key, value, "Put")
}

func (ck *Clerk) Append(key string, value string) {
	ck.PutAppend(key, value, "Append")
}
```

- :two: 服务器

服务器都是先封装 Op, 然后通过调用 Start() 交给底层的 Raft, 若发现自己不再是 leader 的话直接返回; 否则开启定时器, 若未在规定时间内收到命令以完成的通知的话回复 `ErrWrongLeader`; 否则根据命令的执行结果设置 `reply`。注意根据提示的要求最后需要释放内存

**注意所有 `Get request` 都执行, `Put / Append request` 执行前需要判断是否为过期的 RPC 或已经执行过的命令**

```go
func (kv *KVServer) Get(args *GetArgs, reply *GetReply) {
    // 封装Op
	index, term, isLeader := kv.rf.Start(op)
    // 非 Leader, 设置 reply.Err = ErrWrongLeader 并返回
    // 初始化 waitChan ( kv.waitChMap[index] )
    // 等待直到定时器超时或收到 waitChan 的通知
	select {
	case res := <-waitChan:
		reply.Value, reply.Err = res.Value, OK
        // 检查是否仍未当时的 Leader, 提示 5 中说明了这种情况
		currentTerm, stillLeader := kv.rf.GetState()
		if !stillLeader || currentTerm != term {
			reply.Err = ErrWrongLeader
		}
	case <-time.After(ExecuteTimeout):
		reply.Err = ErrWrongLeader
	}
    delete(kv.waitChMap, index)
}

func (kv *KVServer) PutAppend(args *PutAppendArgs, reply *PutAppendReply) {
	// Put或Append 需要先检查是否为过期的 RPC 或已经执行过的命令，避免重复执行
    // 封装Op
	index, term, isLeader := kv.rf.Start(op)
    // 非 Leader, 设置 reply.Err = ErrWrongLeader 并返回
    // 初始化 waitChan ( kv.waitChMap[index] )
    // 等待直到定时器超时或收到 waitChan 的通知
	select {
	case res := <-waitChan:
        reply.Err = OK
        // 检查是否仍未当时的 Leader, 提示 5 中说明了这种情况
		currentTerm, stillLeader := kv.rf.GetState()
		if !stillLeader || currentTerm != term {
			reply.Err = ErrWrongLeader
		}
	case <-time.After(ExecuteTimeout):
		reply.Err = ErrWrongLeader
	}
	delete(kv.waitChMap, index)
}
```

#### :cherries: 其它函数

```go
// 监听 Raft 提交的 applyMsg, 根据 applyMsg 的类别执行不同的操作
// 为命令的话，必执行，执行完后检查是否需要给 waitCh 发通知
func (kv *KVServer) applier() {
	for !kv.killed() {
		applyMsg := <-kv.applyCh
		kv.mu.Lock()
		// 根据收到的是命令还是快照来决定相应的操作
		if applyMsg.CommandValid {
			op := applyMsg.Command.(Op)
			kv.execute(&op)
			currentTerm, isLeader := kv.rf.GetState()
			// 若当前服务器已经不再是 leader 或者是发生了分区的旧 leader，不需要通知回复客户端
			// 指南中提到的情况：Clerk 在一个任期内向 kvserver 领导者发送请求、等待答复超时并在另一个任期内将请求重新发送给新领导者
			if isLeader && applyMsg.CommandTerm == currentTerm {
				kv.notifyWaitCh(applyMsg.CommandIndex, &op)
			}
		} else if applyMsg.SnapshotValid {
			//
		}
		kv.mu.Unlock()
	}
}

// 执行命令，Get 必执行；非 Get 需检查当前命令是否有效，执行后更新该客户端最近一次请求的Id
func (kv *KVServer) execute(op *Op) {
	// 因为是执行完后才会更新，可能会有重复命令在第一次检查时认为不是重复命令，然后被执行了两遍
	// 所以在真正执行命令前需要再检查一遍，若发现有重复日志并且操作不是 Get 的话直接返回 OK
	if op.OpType != "Get" && kv.isInvalidRequest(op.ClientId, op.RequestId) {
		return
	} else {
		switch op.OpType {
		case "Get":
			op.Value = kv.KVDB[op.Key]
		case "Put":
			kv.KVDB[op.Key] = op.Value
		case "Append":
			str := kv.KVDB[op.Key]
			kv.KVDB[op.Key] = str + op.Value
		}
		kv.UpdateLastRequest(op)
	}
}

// 给 waitCh 发送通知，让其生成响应
func (kv *KVServer) notifyWaitCh(index int, op *Op) {
	if waitCh, ok := kv.waitChMap[index]; ok {
		waitCh <- op
	}
}

// 检查当前命令是否为无效的命令 (非 Get 命令需要检查，可能为过期的 RPC 或已经执行过的命令)
func (kv *KVServer) isInvalidRequest(clientId int64, requestId int64) bool {
	if lastRequestId, ok := kv.lastRequestMap[clientId]; ok {
		if requestId <= lastRequestId {
			return true
		}
	}
	return false
}

// 更新对应客户端对应的最近一次请求的Id, 这样可以避免今后执行过期的 RPC 或已经执行过的命令
func (kv *KVServer) UpdateLastRequest(op *Op) {
    lastRequestId, ok := kv.lastRequestMap[op.ClientId]
    if (ok && lastRequestId < op.RequestId) || !ok {
        kv.lastRequestMap[op.ClientId] = op.RequestId
    }
}
```

### :sob: 遇到的一些问题

Lab 3A 还好, 并没有像 Lab 2 一样遇到了很多 bug, 然后不断的打日志, 找 bug, 改 bug。主要原因还是 Lab 3 的提示部分和实验说明部分给出了很多种特殊情况 (要求你在即使发生了这些情况时也能正常工作), 所以写代码的时候就重点关注了这些情况

#### :one: 注意只要是从 applyCh 中取得的命令一定会走到 “执行” 那一步

之前的实现中会先判断此时 Raft 的任期和 applyMsg 中的任期是否一致, 若不一致则表明发生了 Leader 的变更, 放弃该命令。但是应该注意到只要是达成共识的命令必须得 “执行” !! 否则维护的键值数据库中的数据会出问题, 不满足线性一致性

**“执行”** 表明的是走到执行那一步, 若发现是过期的 RPC 或已经执行过的命令当然不会对 `Put 或 Append` 执行两次

其它如当前 KVServer 底层的 Raft 不再是 Leader 或者 底层的 Raft 又重新当选了 Leader (但是任期发生了改变) 等异常情况**影响的是当前 KVServer 是否需要给 `waitCh` 发通知而不是是否执行命令**，发通知表明希望 KVServer 给 Client 响应!!

#### :two: 3A 的第二个测试案例

这个测试案例会发送 1000 条 `Append` 请求, 计算你的代码完成所有请求消耗的时间, 并设置了下限为每次心跳需完成三个请求。测试中的心跳为 100ms, 所以报的错为

> --- FAIL: TestSpeed3A (100.39s)
test_test.go:419: Operations completed too slowly 100.00975ms/op > 33.333333ms/op

检查了一遍 Lab 3A 代码后就意识到了这并不是上层服务出现的问题, 根本问题在于底层 Raft 同步命令太慢了。在原先的实现中心跳仅会在 Candidate 成为 Leader 的时候或者每次心跳定时器到 100ms 后发送心跳, 而请求又是顺序发送的, 必须等待上一个请求完成后才会发送下一个请求。因此原先的实现中若忽略 RPC 的调用等时间, 最理想的情况也是一次心跳完成一个请求, 所以最终耗时大于 1000 * 100ms = 100s; 为了加速响应, 应当在 Leader 收到新命令后就立即发送一次心跳。因此修改了 Raft 的部分代码, 在 `Start()` 函数内, 增加了一旦新命令被添加到日志后就立即发送心跳进行同步的逻辑, 修改后就通过了该测试用例

#### :three: waitCh 的检查

因为实验要求中明确规定了要能即时释放内存, 所以创建的 `waitCh` 在 `Get(), PutAppend()` 结束的时候应该进行关闭 (实现中采用 `map` 来保存日志下标和管道的映射关系, 所以需要调用 `delete` 删除这个 `map` 中的键); 但是因为处理 RPC 的协程跟接收 `applyMsg`, 执行 Op 并给 `waitCh` 发送通知的协程并不是一个协程, 所以很可能出现处理 RPC 的函数超时后关闭了 `waitCh` 但是另一个协程之后才执行完 Op 并给 `waitCh` 发通知的情况。向一个已关闭的通道写数据会引发 panic, 所以在发送前需要检查该 `waitCh` 是否存在

此外, 监听 `applyCh` 的协程收到 `applyMsg` 后会执行 Op, 给 `waitCh` 发通知; 然后继续读 `applyCh`, 因此不能阻塞, `waitCh` 在初始化时必须给予缓冲区, 大小为 1 即可

### :rainbow: 结果

![3A结果](https://github.com/casey-li/MIT6.5840/blob/main/Lab3/Lab3A/result/pic/Lab3A%E7%BB%93%E6%9E%9C.png?raw=true)

通过了 500 次的压力测试，结果保存在 `Lab3A/result/test_3A_500times.txt` 中

---

## :wink: Lab 3B - Key/value service with snapshots

在 Lab 3A 中, 键/值服务器不会调用 Raft 库的 `Snapshot()` 方法, 因此重新启动的服务器必须读取完整的持久 Raft 日志才能恢复其状态, 为了节省日志空间并减少重新启动时间, Lab3B 将使用 Lab 2D 中的 `Snapshot()` 方法

### :cherry_blossom: 目标

修改键/值服务器以便它能够检测到持久化 Raft 状态何时变得太大, 然后通过与 Raft 库的 `Snapshot()` 方法合作，实现快照功能，以减少重新启动时间并节省日志空间; 当键/值服务器重新启动时，它应该从 `persister` 读取快照并从快照恢复其状态

### :mag: 提示

测试时会给 `StartKVServer()` 传递 `maxraftstate` 参数, 它表明持久化的 Raft 状态的最大以字节数 (包括日志，但不包括快照)。因此若调用 `persister.RaftStateSize()` 后发现当前持久化状态大小接近阈值 `maxraftstate` 的话, 就应该调用 Raft 的 `Snapshot()` 来保存快照; 若 `maxraftstate` 为 -1, 表明不必创建快照

- :one:
- :two: 
- :three: 
- :four: 
- :five: 

### :pizza: 数据结构

### :beers: 实现

### :rainbow: 结果

---


---
# :rose: 参考

[博客1](https://blog.csdn.net/qq_40443651/article/details/117172246)
[博客2](https://www.codercto.com/a/84326.html)