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

**Lab 3A 和 Lab 3B 都通过了 1000 次的压力测试 (500次单独的测试, 500次最终测试)**

### 一致性概念

**CAP原理**: CAP (一致性、可用性、分区容忍性) 其实是一种权衡平衡的思想, 用于指导在系统可用性设计。一致性强调在同一时刻副本一致, 可用性指的是服务在有限的时间内完成处理并响应, 分区容忍性说的是分布式系统本身的特点可以独立运行并提供服务

**分布式数据库必须要有 P (Partition Tolerant 分区容忍性), 所以主要是在 C (Consistent 一致性) 和 A (Available 可用性) 之间做选择**

分布式系统中的一致性按照从强到弱可以分为四种 ([分布式系统一致性 - 总结](https://zhuanlan.zhihu.com/p/57315959))

- :one: 线性一致性 (Linearizability)
- :two: 顺序一致性 (Sequential consistency)
- :three: 因果一致性 (Causal consistency)
- :four: 最终一致性 (Eventually Consistency)

#### 线性一致性

要求系统表现的如同一个单一的副本, 按照实际的时间顺序来串行执行线程的读写操作, 满足
- :one: 每一个读操作都将返回『最近的写操作』 (基于单一的实际时间) 的值
- :two: 对任何 client 的表现均一致

**横轴为实际时间**

![线性一致性案例](https://github.com/casey-li/MIT6.5840/blob/main/Lab3/result/%E7%BA%BF%E6%80%A7%E4%B8%80%E8%87%B4%E6%80%A7%E6%A1%88%E4%BE%8B.png?raw=true)

最开始 P1 执行了写 x 为 3 的操作 (肯定是在 invocation 和 response 之间生效, 但具体生效时间不确定); 从 P2 第一次读操作结果为 0 可知此时并未生效, 从 P3 读操作返回 3 可知在 P2 调用 Inv(R, x) 到 给 P3 生成响应这段时间里 P1 的 Inv(w, x, 3) 生效了。根据线性一致性的要求, 所有在 P2 Res(R, x, 3) 时间点往后读 x 的操作都应该返回 3 !!

因此若 P2 再次调用 Inv(R, x) 得到的 ? 是 0, 就不满足线性一致性 (对外表现的并不像单一的副本, P3 访问的服务器已经同步了 P1 的写操作但 P2 访问的服务器此时没来的及更新结果); 若 ? 是 3 则满足线性一致性

#### 顺序一致性

相比于线性一致性, 顺序一致性放松了一致性的要求, 它并不要求操作的执行顺序严格按照真实的时间序; 满足
- :one: 从单个线程或者进程的角度上看, 其指令的执行顺序以编程中的顺序为准
- :two: 从所有线程或者进程的角度上看, 指令的执行保持一个单一的顺序

总之就是要求对于一个进程的所有指令而言, 必须按照规定的顺序执行; 对于所有进程而言, 它们的操作需满足原子性; 可以抽象为有多个进程在访问一个临界资源, 同一时刻仅有一个进程能够对临界资源进行操作 (只能按照程序中规定好的顺序依次对该共享资源进行处理), 执行完一个操作后所有进程再次开始争抢该临界资源的访问权

**横轴表示程序内部的执行顺序**

![顺序一致性案例1](https://github.com/casey-li/MIT6.5840/blob/main/Lab3/result/%E9%A1%BA%E5%BA%8F%E4%B8%80%E8%87%B4%E6%80%A7%E6%A1%88%E4%BE%8B.png?raw=true)

上面的两种执行结果均满足顺序一致性, 只要按照 `P1 x.write(2) -> P1 x.write(3) -> P2 x.write(5) -> P1 x.read(5)` 的先后运行顺序执行即可

![顺序一致性案例2](https://github.com/casey-li/MIT6.5840/blob/main/Lab3/result/%E9%A1%BA%E5%BA%8F%E4%B8%80%E8%87%B4%E6%80%A7%E6%A1%88%E4%BE%8B2.png?raw=true)

这个图就不满足顺序一致性的要求, 因为不管进程之间怎么执行都不可能出现 P3, P4 连续读两次的结果相反的情况

#### 线性一致性和顺序一致性的对比

|线性一致性|顺序一致性|
| :--: | :--: |
| 单一进程要按照时间序执行 | 单一进程要按照程序规定的顺序执行 |
| 不同进程要按照时间序执行 | 不同进程的执行顺序无要求 |

**示意图**

![线性一致性和顺序一致性的对比图](https://github.com/casey-li/MIT6.5840/blob/main/Lab3/result/%E7%BA%BF%E6%80%A7%E4%B8%80%E8%87%B4%E6%80%A7%E5%92%8C%E9%A1%BA%E5%BA%8F%E4%B8%80%E8%87%B4%E6%80%A7%E5%AF%B9%E6%AF%94.png?raw=true)

#### 因果一致性

因果一致性在一致性的要求上, 又比顺序一致性降低了, 它仅要求有因果关系的操作顺序得到保证, 非因果关系的操作顺序则无所谓

因果一致性往往发生在分区 (也称为分片) 的分布式数据库中, 分区后，每个节点并不包含全部数据, 不同的节点独立运行, 因此不存在全局写入顺序。如果用户A提交一个问题, 用户B提交了回答. 问题写入了节点A, 回答写入了节点B, 因为同步延迟, 发起查询的其它用户可能会先看到回答, 再看到问题。为了防止这种异常，就需要满足因果一致性, 即如果一系列写入按某个逻辑顺序发生, 那么任何人读取这些写入时, 会看见它们以正确的逻辑顺序出现

因果一致性一般应用在跨地域同步数据中心系统中, 例如Facebook, 微信这样的应用程序。全球各地的用户, 往往会访问其距离最近的数据中心, 数据中心之间再进行双向的数据同步

#### 最终一致性

虽然 CAP 理论说明了选择了 A 就不可能得到真正的 C, 但是业务系统在大多情况下对一致性没那么高的要求反而更多强调高可用性, 因此只需追求最终一致性即可

只要求每个系统节点总是可用的, 同时任何的写 (修改数据) 操作都会在后台同步给系统的其他节点 (所有节点最终都能取得已更新的数据, 但不能保证其它节点能立即取得已更新的数据); 这意味着在任意时刻, 整个系统是 Inconsistent(不一致的), 然而从概率上讲, 大多数的请求得到的值是准确的

互联网的 DNS (域名服务) 就是最终一致性的一个非常好的例子。你注册了一个域名, 这个新域名需要几天的时间才能通知给所有的 DNS 服务器, 但是不管什么时候, 你能够连接到的任意 DNS 服务器对你来说都是 'Available' 的

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
            // 若当前服务器已经不再是 leader 或者是旧 leader，不需要通知回复客户端
            // 指南中提到的情况：Clerk 在一个任期内向 kvserver 领导者发送请求, 可能在此期间当前领导者丧失了领导地位但是又重新当选了 Leader
            // 虽然它还是 Leader, 但是已经不能在进行回复了，需要满足线性一致性 (可能客户端发起 Get 时应该获取的结果是 0, 但是在次期间增加了 1。若现在回复的话会回复 1, 但是根据请求时间来看应该返回 0)
            // 所以不给客户端响应, 让其超时, 然后重新发送 Get, 此时的 Get 得到的结果就应该是 1 了 (只要任期没变, 都是同一个 Leader 在处理的话, 因为有重复命令的检查, 必定满足线性一致性)
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

修改键/值服务器以便它能够检测到持久化 Raft 状态何时变得太大, 然后调用 Raft 库的 `Snapshot()` 方法实现快照功能，以减少重新启动时间并节省日志空间; 当键/值服务器重新启动时，它应该从 `persister` 读取快照并从快照恢复其状态

### :mag: 提示

测试时会给 `StartKVServer()` 传递 `maxraftstate` 参数, 它表明持久化的 Raft 状态 (自己的 Raft 实现中就是 `currentTerm`, `voteFor` 和 `log` 字段) 的最大以字节数 (包括日志，但不包括快照); 因此若调用 `persister.RaftStateSize()` 后发现当前持久化状态大小接近阈值 `maxraftstate` 的话, 就应该调用 Raft 的 `Snapshot()` 方法来保存快照; 若 `maxraftstate` 为 -1, 表明不必创建快照

- :one: 考虑 KVServer 何时应该调用 `Raft.Snapshot()` 以及快照中应该包含哪些内容; Raft 会调用 `Save()` 将快照和对应的持久化状态存储在持久化对象中; 使用 `ReadSnapshot()` 读取最新存储的快照
- :two: KVServer 必须能够跨检查点检测日志中的重复操作 (自己理解的检查点是生成快照的点, 即要求重启后能识别出当前命令是否在快照中), 用于检测命令是否重复的状态必须被包含在快照中
- :three: 快照中存储的所有字段大写
- :four: Lab 2 的 Raft 库中在本测试案例下可能会暴露出新问题, 若对 Raft 的实现进行了更改, 请确保它能继续通过所有 Lab 2 的测试

### :dart: 思路

相比于 Lab 3A, 增加了快照功能, 但是原先的逻辑都不用更改 (不像 Lab 2D 需要改很多很多东西), 只需要增加部分逻辑即可, 直接调用 `Raft.Snapshot()` 就可以了 (Lab 2D 写好的)。底层 Raft 之间的共识 KVServer 不用管, 所以 Lab 3B 其实并没有新增多少代码

- :one: 考虑 KVServer 何时应该调用 `Raft.Snapshot()`?

实验描述中也说的很清楚了, 当 `persister.RaftStateSize() > maxraftstate` 时就调用 `Raft.Snapshot()`, 而持久化状态的大小跟日志长度相关, 所以应在 KVServer 知道日志长度增加了以后检查条件

KVServer 在两个地方知道底层 Raft 的日志增加了 (调用 `Start()` 和 从 `applyCh` 接收到命令), 但是调用 `Raft.Snapshot()` 必须提供裁剪到哪的下标并且必须确保该下标之前的日志已经被 `apply`, 所以**只能在 KVServer 接收到命令后进行检查并生成快照**

- :two: 应对哪些内容生成快照？

对 Raft 的日志进行裁剪就意味着 KVServer 无法通过读取日志来恢复其数据库内容, 因此 KVServer 必须将**当前键值数据库的内容**生成快照 (`KVDB map[string]string`), 这样即使崩溃了也可以通过读取保存的数据库状态和 Raft 中持久化的日志快速恢复其数据库内容

提示 2 中说了应该在快照中保存用于检测命令是否会被重复执行的信息, 因此还需保存 `LastRequestMap map[int64]int64`

最终效果为 KVServer 通过快照保存自己的数据库信息和所有客户端最近一次的请求信息, 底层 Raft 通过持久化状态保存任期, 投票和日志结果信息

#### 流程图

KVServer 新增快照功能后, 只是多了一个检测是否需要生成快照的逻辑和接收到快照 applyMsg 后如何处理的逻辑, 之前的逻辑基本不变, 一次具体的请求流程如下 ***(新增的逻辑用斜体加粗表示)***

![一次具体的请求流程](https://github.com/casey-li/MIT6.5840/blob/main/Lab3/Lab3B/result/pic/3B%E5%8D%95%E6%AC%A1%E8%AF%B7%E6%B1%82%E7%A4%BA%E6%84%8F%E5%9B%BE.png?raw=true)

***Leader KVServer***

- :one: Client 给 KVServer 的 Leader 发送 `RPC request`
- :two: KVServer 封装 Op 并调用 Raft 的 `Start()` 提交日志, 因为需要在规定时间内给 Client 响应, 会同步开启定时器; 若未能在规定时间内在 `waitCh` 上读取到到相应的结果, 直接返回 `Err`
- :three: 底层 Raft 达成共识 ***(区别是这里 Leader 可能会给 Follower 发送快照以达成共识)***
- :four: Raft `apply` 该命令, 将其发送到 `applyCh`, 此时 KVServer 监听 `applyCh` 的协程会收到 `applyMsg` 
- :five: KVServer 根据 `applyMsg` 中的 Op 执行相应的命令, 给 `waitCh` 发送执行结果。这样正在处理 `RPC request` 的协程 (正在监听 `waitCh` 和定时器) 就会收到信息, 当然前提是此时未达到定时时间; ***(随后检查 Raft 持久化状态大小, 若超过了 `maxraftstate`, 调用 `Raft.Snapshot()` 即可)***
- :six: KVServer 根据 `waitCh` 读取的结果生成 `RPC response`, 响应 Client

还有一个注意点就是初始化的时候记得读取快照 (Snapshot 的编码解码这部分直接参考 Lab 2 写就可以了, 都是模板, 改个需要进行快照的参数就行了)

***Follower KVServer***

- :one: 等待达成共识; 根据 Lab 2 实现的 Raft 可知会慢于 Leader 一拍 (达成共识后, Leader 提交命令, 更新 commitIndex; Follower 会在下一次收到心跳时发现自己的 commitIndex 落后于 Leader 的, 进而提交命令, ***现在也可能在心跳中收到快照进而提交快照***)
- :two: Raft `apply` 该命令, 将其发送到 `applyCh`, 此时 KVServer 监听 `applyCh` 的协程会收到 `applyMsg` 
- :three: ***KVServer 判断 applyMsg 的类别***
    - 若为命令, 根据 `applyMsg` 中的 Op 执行相应的命令, ***(随后检查 Raft 持久化状态大小, 若超过了 `maxraftstate`, 调用 `Raft.Snapshot()` 即可)***
    - ***若为快照, 解析快照, 更新自身状态 (同命令, 需要判断是旧的快照, 不能回退自身状态)***

***总结***

增加快照后, Leader KVServer 和 Follower KVServer 的流程又新增了不同点

Lab 3A 中的区别在于在执行完 Op 后是否需要给 waitCh 发送执行完毕的通知 (Follower KVServer 没有 waitCh, 因为不需要回复客户端);

Lab 3B 的区别在于快照的生成和解析 (同底层的 Raft 一样)。 Leader KVServer 只负责生成快照, 它永远也不会从 applyCh 中收到快照 applyMsg; Follower KVServer 既会在 Raft 的持久化状态大小过大时生成快照, 也会接收来自 Leader 的快照并根据快照内容更新自身状态

### :pizza: 数据结构

相比于 Lab 3A, 仅修改了 KVServer 的结构, 新增加了两个字段

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
    LastRequestMap map[int64]int64   // 保存每个客户端对应的最近一次请求的内容（包括请求的Id 和 回复）

    // 3B 新增字段
    persister         *raft.Persister
    lastIncludedIndex int
}
```

### :beers: 实现
#### :cherries: Snapshot 的编码解码

仿照 Lab 2 写就可以了

```go
// 需要持久化的字段为 数据库
// 为了避免重复执行命令，每个客户端最近一次请求的 Id 也需要持久化处理
func (kv *KVServer) encodeState() []byte {
    w := new(bytes.Buffer)
    e := labgob.NewEncoder(w)
    e.Encode(kv.KVDB)
    e.Encode(kv.LastRequestMap)
    kvstate := w.Bytes()
    return kvstate
}

// 读取持久化状态
func (kv *KVServer) readPersist(data []byte) {
    if data == nil || len(data) < 1 {
        return
    }
    r := bytes.NewBuffer(data)
    d := labgob.NewDecoder(r)
    kvdb := map[string]string{}
    lastRequestMap := map[int64]int64{}
    if d.Decode(&kvdb) != nil || d.Decode(&lastRequestMap) != nil {
        return
    } else {
        kv.KVDB = kvdb
        kv.LastRequestMap = lastRequestMap
    }
}
```
#### :cherries: 主要函数

主要需要修改的就一个 applier() 函数, 新增一个是命令的话检查 Raft 持久化状态大小; 是快照的话读快照, 更新状态

```go
// 监听 Raft 提交的 applyMsg, 根据 applyMsg 的类别执行不同的操作
// 为命令的话，必执行，执行完后检查是否需要给 waitCh 发通知
// 为快照的话读快照，更新状态
func (kv *KVServer) applier() {
    for !kv.killed() {
        applyMsg := <-kv.applyCh
        kv.mu.Lock()
        if applyMsg.CommandValid {
            // 3B 需要判断日志是否被裁剪了
            if applyMsg.CommandIndex <= kv.lastIncludedIndex {
                kv.mu.Unlock()
                continue
            }

            op := applyMsg.Command.(Op)
            kv.execute(&op)
            currentTerm, isLeader := kv.rf.GetState()
            // 若当前服务器已经不再是 leader 或者是旧 leader，不需要通知回复客户端
            // 指南中提到的情况：Clerk 在一个任期内向 kvserver 领导者发送请求, 可能在此期间当前领导者丧失了领导地位但是又重新当选了 Leader
            // 虽然它还是 Leader, 但是已经不能在进行回复了，需要满足线性一致性 (可能客户端发起 Get 时应该获取的结果是 0, 但是在次期间增加了 1。若现在回复的话会回复 1, 但是根据请求时间来看应该返回 0)
            // 所以不给客户端响应, 让其超时, 然后重新发送 Get, 此时的 Get 得到的结果就应该是 1 了 (只要任期没变, 都是同一个 Leader 在处理的话, 因为有重复命令的检查, 必定满足线性一致性)
            if isLeader && applyMsg.CommandTerm == currentTerm {
                kv.notifyWaitCh(applyMsg.CommandIndex, &op)
            }

            // 3B 执行完命令后检查状态, 有必要的化执行快照压缩 Raft 的日志
            if kv.maxraftstate != -1 && kv.persister.RaftStateSize() >= kv.maxraftstate {
                kv.rf.Snapshot(applyMsg.CommandIndex, kv.encodeState())
            }
            kv.lastIncludedIndex = applyMsg.CommandIndex

        } else if applyMsg.SnapshotValid {
            // 一定是 Follower 收到了快照, 若是最新的快照, 读取快照信息并更新自身状态
            if applyMsg.SnapshotIndex <= kv.lastIncludedIndex {
                kv.mu.Unlock()
                continue
            }
            kv.readPersist(applyMsg.Snapshot)
            kv.lastIncludedIndex = applyMsg.SnapshotIndex
        }
        kv.mu.Unlock()
    }
}
```

另一个修改的地方是初始化的时候记得读快照

### :sob: 遇到的一些问题

最开始测试的时候就产生死锁了, 检查代码后发现问题在于读快照部分 (因为是原封不动的拿的 Lab 2 中编码解码代码也没多想 :sob:) 
在 Lab 2 的 Raft 的实现中, 自己想着读取持久化状态会修改共享资源就给加了锁 (这在 Raft 里是没啥问题的, 因为读取持久化状态仅仅发生在 Raft 初始化的时候, 此时不会产生死锁); 但是在 KVServer 的实现流程中, 当从 applyCh 中接收到的 applyMsg 为 Snapshot 时 (此时已经加锁), 就会读取 Snapshot, 在加锁就发生了死锁!!


### :rainbow: 结果

![3B 结果](https://github.com/casey-li/MIT6.5840/blob/main/Lab3/Lab3B/result/pic/Lab3B%E7%BB%93%E6%9E%9C.png?raw=true)

通过了 500 次的压力测试，结果保存在 `Lab3B/result/test_3B_500times.txt` 中

---

## 最终结果

![Lab 3 结果](https://github.com/casey-li/MIT6.5840/blob/main/Lab3/result/Lab3%E7%BB%93%E6%9E%9C.png?raw=true)

通过了 500 次的压力测试，结果保存在 `Lab3/result/test_3_500times.txt` 中

---

# :rose: 参考
[分布式系统一致性 - 总结](https://zhuanlan.zhihu.com/p/57315959)

[MIT 6.824 Lab3 2021 AB完成记录](https://blog.csdn.net/qq_40443651/article/details/117172246)

[Lab3A. 基于 Raft 实现容错的 kvDB](https://www.codercto.com/a/84326.html)

