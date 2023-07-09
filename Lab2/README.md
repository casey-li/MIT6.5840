# :two_hearts: Lab 2、Raft

[实验介绍](https://pdos.csail.mit.edu/6.824/labs/lab-raft.html)

Lab2 系列为 Raft 分布式一致性协议算法的实现，Raft 将分布式一致性共识分解为若干个子问题
- leader election，领导选举 (Lab 2A)
- log replication，日志复制 (Lab 2B)
- safety，安全性(Lab 2B & 2C)；2C 除了持久化还有错误日志处理

以上为 raft 的核心特性，除此之外，要用于生产环境，还有许多地方可以优化
- log compaction，日志压缩-快照(lab2D)
- Cluster membership changes，集群成员变更

**:mag: 提示** 

**注意后序实验都是基于前面的实验完成的, 所以一定要确保前面的实验没问题, 跑测试用例的时候多跑几次, 自己是在做 Lab2C 的时候吃了亏, 又重新回来改的 Lab2A 和 Lab2B, 目前每个 Lab 都是跑了 1000 次无错的结果**

下面的 Lab2A 和 Lab2B 存在一些小问题 (自己并未修改这部分的描述, 只改了代码。主要是为了回看当时的思路, 方便对比, 知道为什么会有问题)。存在的问题都在 Lab2C 中的 bugs 小节进行了详细描述, 给出了出错原因, 在什么情况下会出错以及解决方案。

## :wink: Lab 2A - leader election

### :cherry_blossom: 目标
实现 Raft 的 leader election 和 heartbeats, 注意是没有日志条目的 `AppendEntries` RPC

### :maple_leaf: 效果
选出一个单一的领导者，如果没有瘫痪，领导者继续担任领导者，如果旧领导者瘫痪或旧领导者的数据包丢失，则由新领导者接管丢失

### :mag: 提示
- :one: 按照论文的 Fig.2 实现发送和接收 `RequestVote` RPC
- :two: 根据 Fig.2 完善 Raft 和两个 RPC 相关的结构体, 修改 `Make()` 以创建后台 goroutine。当它有一段时间没收到其他对等点的消息时, 发送 `RequestVote` RPC 来定期启动领导者选举
- :three: 实现 `heartbeats`, leader 定期发送 `AppendEntries` RPC 以重置其他节点的选举开始时间
- :four: 确保不同节点的选举超时时间不都相等, 避免它们只为自己投票, 导致没有人能成为 leader
- :five: 测试程序中规定了 leader 每秒发送 `heartbeats` 的次数小于10次, 并且要求在旧领导失败后的 5s 内必须选出新的领导者，因此必须使用比论文中 150 - 300 ms 更大的选举超时时间, 但也不能太大
- :six: [指南页](https://thesquareplanet.com/blog/students-guide-to-raft/)有很多提示, 有助于完成实验
- :seven: 不要忘记实现 `GetState()` 方法
- :eight: Go RPC 只发送以大写字母开头的结构体字段, 子结构也必须有大写字段名, 注意结构体中的实现

#### :heavy_exclamation_mark: :heavy_exclamation_mark: :heavy_exclamation_mark: Fig.2, 最重要的一张图

![Fig 2](https://github.com/casey-li/MIT6.5840/blob/main/Lab2/Lab2A/result/pic/Raft_Fig2.png?raw=true)


### :lollipop: 各个角色的职责

每个角色 (leader, follower 和 candidate) 都有一个后台的定时器, 本实验中 leader 的定时器设为了 100ms, follower 和 candidate 设为了 [250, 400] 之间的随机数

- :one: follower 和 leader 的定时器结束后会检查条件 (因为在该期间内所有节点的状态可能已经发生了改变), 若一切正常则可以开始选举, 发送请求投票, 根据响应做下一步处理
- :two: leader 的定时器结束后同样需要检查它自己的状态, 若仍为 leader就可以给其它所有节点发送心跳, 等待响应,根据响应做下一步处理

RPC 请求

- :one: leader 负责周期性地广播发送 `AppendEntries` RPC 请求
- :two: candidate 负责周期性地广播发送 `RequestVote` RPC 请求
- :three: follower 仅负责被动地接收 RPC 请求，从不主动发起请求

`AppendEntries` RPC, 用于**发送心跳**和**复制日志 (Lab2A 暂时不用管)**

`RequestVote` RPC, 用于候选人拉票

### :pizza: 数据结构
看 Fig.2 即可, 给的很清楚了, 注释的是目前 Lab2B 用不到的
```go

type Raft struct {
	mu        sync.Mutex          // Lock to protect shared access to this peer's state
	peers     []*labrpc.ClientEnd // RPC end points of all peers
	persister *Persister          // Object to hold this peer's persisted state
	me        int                 // this peer's index into peers[]
	dead      int32               // set by Kill()
	currentTerm int
	voteFor     int
	state       RuleState
	// log         []LogEntry
	// commitIndex int
	// lastApplied int
	// nextIndex   []int
	// matchIndex  []int

	// 自己加的, 选举定时器开始时间, 用于跟当前时间计算差值
	electionStartTime time.Time
}

type RuleState int

const (
	Follower RuleState = iota
	Candidate
	Leader
	Dead
)

// AppendEntries RPC 的参数和回复结构体
type AppendEntriesArgs struct {
	Term         int
	LeaderId     int
	// PrevLogIndex int
	// PrevLogTerm  int
	// Entries      []LogEntry
	// LeaderCommit int
}

type AppendEntriesReply struct {
	Term    int
	Success bool
}

// RequestVote RPC 的参数和回复结构体
type RequestVoteArgs struct {
	Term         int
	CandidateId  int
	// LastLogIndex int
	// LastLogTerm  int
}

type RequestVoteReply struct {
	Term        int
	VoteGranted bool
}
```

### :beers: 实现

**:warning: :warning: :warning: 注意锁, 因为死锁找了很久的 Bug**

**多打日志看各个节点的信息**


#### :cherries: 主要函数

```go
// 根据状态运行定时器
func (rf *Raft) ticker() {
	if !rf.killed() {
		rf.mu.Lock()
		state := rf.state
		rf.mu.Unlock()
		switch state {
		case Follower:
			rf.runElectionTimer()
		case Candidate:
			rf.runElectionTimer()
		case Leader:
			rf.heartBeatsTimer()
		}
	}
}

// 选举定时器
func (rf *Raft) runElectionTimer() {
	// 1 设置超时时间, 记录当前节点的任期 nowTerm
	
	// 2 利用 time.NewTicker() 不断循环检查, 自己设置的每 10 ms 检查一次
	for {

		// 2.1 若变成了 leader, 直接返回
		
		// 2.2 若 nowTerm != rf.currentTerm, 表明这是上一个任期的定时器,直接返回

		// 2.3 检查是否超时, 若超时了则转到 startElection() 开始选举
	}
}

// 开始选举, 发送 RequestVote RPC 请求投票
func (rf *Raft) startElection() {
	// 1 修改状态, 增加任期, 给自己投票并重设定时器开始时间

	// 2 开启多个协程发送 RequestVote RPC
	for peerId := range rf.peers {
		go func(peerId int) {
			
			// 2.1 设置 RequestVoteArgs 参数, 发送 RequestVote RPC, 等待响应

			// 2.2 检查状态, 若当前已经不是 Candidate, 直接退出

			// 2.3 检查 reply.Term, 若比自己的 term 更大, 调用 becomeFollower(reply.Term) 并退出

			// 2.4 若 term 相等并且是赞同票, 累加得票数, 若超过半数调用 becomeLeader()

		}(peerId)
	}

	// 3 启动新的选举定时器以防止选举失败, 原来的选举定时器会因为任期的关系自动退出
}


// 修改状态，任期，更新选举开始时间，清除投票结果
func (rf *Raft) becomeFollower(term int) {
    // go rf.ticker() 运行新的选举定时器
}

// 修改状态，go rf.ticker() 运行新的心跳定时器, 原来的 ticker() 会因为设置的条件而主动退出
func (rf *Raft) becomeLeader() {
}

// 心跳定时器
func (rf *Raft) heartBeatsTimer() bool {
	// 设置定时器, 100ms
	for {
		rf.runHeartBeats()
		<-ticker.C
		// 检查状态是否改变, 若不是 leader 的话退出
	}
}

// 心跳函数, 向其他节点发送 AppendEntries RPC
func (rf *Raft) runHeartBeats() {
	// 1 检查状态, 若不是 leader 的话直接退出 (可能是旧 leader 调用的,), 记录当前任期 nowTerm

	// 2 开启多个协程发送 AppendEntries RPC
	for peerId := range rf.peers {
		go func(peerId int) {
			
			// 2.1 等待 AppendEntriesReply, 若调用失败直接退出

			// 2.2 检查 reply.Term, 若比自己的 term 更大, 调用 becomeFollower(reply.Term) 并退出

		}(peerId)
	}
}
```

#### :cherries: RequestVote RPC 和 AppendEntries RPC

```go
// RequestVote RPC
func (rf *Raft) RequestVote(args *RequestVoteArgs, reply *RequestVoteReply) {
	// 1 若状态为 Dead, 直接返回

	// 2 若 args.Term 更大, 调用 becomeFollower(args.Term)

	// 3 当且仅当任期相等 并且 当且节点尚未投票或者本来就投给的该候选者时, 投赞同票并更新选举定时器开始时间, 否则直接投反对票
}

func (rf *Raft) sendRequestVote(server int, args *RequestVoteArgs, reply *RequestVoteReply) bool {
	ok := rf.peers[server].Call("Raft.RequestVote", args, reply)
	return ok
}

// AppendEntries RPC
func (rf *Raft) AppendEntries(args *AppendEntriesArgs, reply *AppendEntriesReply) {
	// 1 若状态为 Dead, 直接返回

	// 2 若 args.Term 更大, 调用 becomeFollower(args.Term) 更新任期等信息

	// 3.1 若任期仍不等 (当前任期更大) 回复 false 和当前任期, leader 收到后会变为 follower
	
	// 3.2 任期相等但是当前不是 follower 的话,调用 becomeFollower(args.Term) 变成 follower, 更新选举定时器的开始时间, 回复 true 和 当前任期
}

func (rf *Raft) sendHeartBeats(server int, args *AppendEntriesArgs, reply *AppendEntriesReply) bool {
	ok := rf.peers[server].Call("Raft.AppendEntries", args, reply)
	return ok
}
```
### :rainbow: 结果

![实验结果](https://github.com/casey-li/MIT6.5840/blob/main/Lab2/Lab2A/result/pic/Lab2A%E7%BB%93%E6%9E%9C.png?raw=true)

通过了 1000 次的连续测试 (`Lab2A/result/test_2A_500times.txt` 和 `Lab2A/result/test_2A_500times_2.txt`)

---

## :wink: Lab 2B - Log

### :cherry_blossom: 目标
日志复制, 实现 leader 和 follower 的相关代码以实现日志追加

### :mag: 提示
- :one: 实现 `Start()`。编写代码, 通过 `AppendEntries` RPC 发送和接收新的日志条目 (参考 Fig.2 )。所有节点都通过 `applyCh` 发送最新提交的日志条目 
- :two: 实现选举限制 (论文中的 5.4.1 节), 具体指在 Lab2A 的基础上, Follower 仅给日志至少跟自己一样新的 Candidate 投赞成票
- :three: 若在测试中发现自己的代码即使在 leader 还活着时也迟迟不能达成一致, 不断进行反复的选举的话, 建议寻找定时选举定时器中的 bug 或者检查 Candidate 在赢得选举后是否没有立即发送心跳
- :four: 代码中若存在不断循环检查某些状态的情况, 不要让它们不断执行, 因为会减慢实现速度, 导致测试失败. 可以使用条件变量或者插入一个时间让其休眠一段时间
- :five: 若测试失败, 请查看 config 中的代码了解该测试在做什么, 这有助于定位 bug

![2B](https://github.com/casey-li/MIT6.5840/blob/main/Lab2/Lab2B/result/pic/Raft2B.png?raw=true)

### :pizza: 数据结构

同Lab2A, 即 Fig.2 中给出的所有字段, 只不过 Lab2B 将使用所有字段

```go
type Raft struct {
	mu        sync.Mutex          
	peers     []*labrpc.ClientEnd 
	persister *Persister          
	me        int             
	dead      int32              
	currentTerm int
	voteFor     int
	state       RuleState

	log         []LogEntry  // 当前节点的日志
	commitIndex int         // 已知要提交的
	lastApplied int         // 已经应用到状态机上的
	nextIndex   []int       // 仅 leader 使用, 保存要发送给其它节点的下一个日志下标
	matchIndex  []int       // 仅 leader 使用, 保存其它服务器上已经提交的日志下标

	electionStartTime time.Time

	applyChan           chan ApplyMsg  // 提交日志的通道
	notifyNewCommitChan chan struct{}  // 用于通知当前有新的日志可被提交的通道，另一个协程在收到通知后利用 applyChan 提交日志
}

type LogEntry struct {
	Command interface{} // 命令
	Term    int			// 当前任期
}

type ApplyMsg struct {
	CommandValid bool			// 是否包含最新提交的日志条目
	Command      interface{}	// 提交的命令
	CommandIndex int			// 命令在日志中的下标
}

type AppendEntriesArgs struct {
	Term         int
	LeaderId     int

	PrevLogIndex int        // leader 中保存的 nextIndex[peerID] - 1
	PrevLogTerm  int        // leader 中保存的 log[PrevLogIndex].Term
	Entries      []LogEntry // 发送的日志, 即 log[nextIndex[peerID] : ]
	LeaderCommit int        // 当前 leader 的 commitIndex
}

type RequestVoteReply struct {
	Term        int
	VoteGranted bool
}

type RequestVoteArgs struct {
	Term         int
	CandidateId  int
	LastLogIndex int // candidate 的 len(log) - 1
	LastLogTerm  int // candidate 的 log[LastLogIndex].Term
}

type AppendEntriesReply struct {
	Term    int
	Success bool
}
```
- Raft 中的 `nextIndex[]` 为 leader 保存的对应 follower 的下一个需要传输的日志条目
- Raft 中的 `matchIndex[]` 为 leader 保存的对应 follower 跟自己一致的最大日志条目下标。 在 leader 收到有效的 `AppendEntries` RPC 回复时, 同时更新 `nextIndex[peerID]` 和 `matchIndex[peerID]` `(nextIndex[peerID] += len(args.Entries), matchIndex[peerID] = nextIndex[peerID] - 1)`

当一个节点变成 leader 后, 重新初始化 `nextIndex[]` 为它的日志长度, 即之后发送 `AppendEntries` RPC 的话从最后一个日志条目开始检查

`commitIndex, lastApplied, nextIndex[], matchIndex[]` 共同组成了 leader 的提交规则. leader 总是最先提交的, 可以认为 leader 为这个集群的代表, leader 提交后, follower 才会提交


> :lollipop: 提交新命令需要多少次 RPC 往返？

:thought_balloon: 两个。第一轮领导者将下一个日志条目发送给追随者，并让追随者确认它们。当领导者处理对 AE 的回复时，它可以根据响应更新其提交索引。第二轮将向关注者发送更新的提交索引，然后关注者将这些条目标记为已提交并将它们发送到提交通道。


### :beers: 实现

**:warning: :warning: :warning: 注意通道需要初始化, 否则直接提交数据会阻塞**
**:warning: :warning: :warning: 注意检查传输的日志的范围**

#### :cherries: 主要函数
主要在 Lab2A 的基础上补充有关日志的信息即可

```go
// 监听 notifyNewCommitChan 是否有通知, 进而向 rf.applyChan 提交命令。由创建 Raft 时启动协程进行监听
func (rf *Raft) commitCommand() {
	for range rf.notifyNewCommitChan {
		// 1. 记录 rf.lastApplied 的值 savedLastApplied, 保存 log[savedLastApplied + 1 : rf.commitIndex + 1] 并更新 rf.lastApplied
		// 2. 依次提交每个日志条目, savedLastApplied 用于设置 CommandIndex
	}
}

// 接收发来的命令，返回 (命令被提交的索引，当前 term，当前机器是否是 leader)
// 若不是 leader, 直接返回 -1, -1, false
// 是 leader 的话, 在自己的日志后面追加一个条目, 返回结果
// 注意是直接返回，因此并不能保证当前命令会被提交
func (rf *Raft) Start(command interface{}) (int, int, bool) {
}

// 开始选举, 发送 RequestVote RPC 请求投票
// Lab2B 仅在发送 RequestVoteArgs 时增加了 LastLogIndex 和 LastLogTerm
// Lab2B 注意选举成功运行 rf.becomeLeader() 时更新 rf.nextIndex[]
func (rf *Raft) startElection() {
}

// 心跳函数, 向其他节点发送 AppendEntries RPC
// Lab2B 中需要完善发送的 AppendEntriesArgs, 发送完整信息。在收到请求后, 应该统计结果来决定是否增加 commitIndex
func (rf *Raft) runHeartBeats() {
	// 1 检查状态, 若不是 leader 的话直接退出 (可能是旧 leader 调用的,), 记录当前任期 nowTerm

	// 2 开启多个协程发送 AppendEntries RPC
	// Lab2B 中需要发送 PrevLogIndex, PrevLogTerm, Entries, LeaderCommit
	for peerId := range rf.peers {
		go func(peerId int) {
			for !rf.killed() {

				// 2.1 等待 AppendEntriesReply, 若调用失败直接退出
	
				// 2.2 检查 reply.Term, 若比自己的 term 更大, 调用 becomeFollower(reply.Term) 并退出
	
				if rf.state == Leader && currentTerm == reply.Term {
					if reply.Success {
						// Lab2B 2.3.1 更新 rf.nextIndex[peerId], rf.matchIndex[peerId]
	
						// Lab2B 2.3.2 保存当前的 commitIndex (savedCommitIndex), 若有半数的节点的 matchIndex[j] 大于 commitIndex, 更新 commitIndex
	
						// Lab2B 2.3.3 若 commitIndex > savedCommitIndex, 给 notifyNewCommitChan 发送通知，提交新的日志条目
					} else {
						// Lab2B 2.3.4 nextIndex[peerId] -= 1
						// Lab2B 2.3.5 重发AppendEntries RPC
						continue;
					}
				}
			}
		}(peerId)
	}
}
```

#### :cherries: RequestVote RPC 和 AppendEntries RPC

在原有基础上增加对日志条目的判断

:one: `RequestVote` RPC 
- 需要检查 Candidate 发来的请求中的最后一个日志条目的任期和下标, 仅给日志比自己更新的 Candidate 投赞同票

:two: `AppendEntries` RPC 
- 需要检查 leader 发来的日志参数, 当且仅当参数中的上一个日志下标小于自己的日志长度并且任期相同时回复 true, 进行后序操作; 否则返回 false 即可, 自己的相关信息不变
- 找到日志不同的地方, 用leader 发来的日志替换自己后序所有日志
- 检查 leader 的 commitIndex 来决定是否更新自己的 commitIndex 并向 notifyNewCommitChan 发送通知

```go
// RequestVote RPC
func (rf *Raft) RequestVote(args *RequestVoteArgs, reply *RequestVoteReply) {
	// 1 若状态为 Dead, 直接返回

	// 2 若 args.Term 更大, 调用 becomeFollower(args.Term)

	/* 
	[Lab2B] 3 当且仅当 (1)任期相等 && (2)当且节点尚未投票或者本来就投给的该候选者 && 
	(3)参数中的上一日志任期更大或者任期相同但是参数中的上一日志的下标不小于自己的时, 
	投赞同票并更新选举定时器开始时间, 否则直接投反对票
	*/
}

// AppendEntries RPC
func (rf *Raft) AppendEntries(args *AppendEntriesArgs, reply *AppendEntriesReply) {
	// 1 若状态为 Dead, 直接返回

	// 2 若 args.Term 更大, 调用 becomeFollower(args.Term) 更新任期等信息

	// 3 若任期仍不等 (当前任期更大) 回复 false 和当前任期, leader 收到后会变为 follower
	if args.Term == rf.currentTerm {
		if rf.state != Follower {
			// 4 任期相等但是当前不是 follower 的话,调用 becomeFollower(args.Term) 变成 follower
		}
		// 5 更新选举定时器的开始时间
		// [Lab2B] 6 检查参数中的上一个日志下标是否小于自己的日志长度并且任期相同, 不满足直接返回 false, 否则之后返回 true
		if args.PrevLogIndex < len(rf.log) && args.PrevLogTerm == rf.log[args.PrevLogIndex].Term {
			// [Lab2B] 7 从 args.PrevLogIndex + 1 开始找日志不同之处 (不断比较直到下标越界或者日志任期不等)

			// [Lab2B] 8 若 argsLogIndex < len(args.Entries) 则将参数中的后序日志拼接到 log[:insertIndex] 后面

			// [Lab2B] 9 若 args.LeaderCommit > rf.commitIndex, 更新 rf.commitIndex 并给 notifyNewCommitChan 发通知
		}
	}
}
```

### :rainbow: 结果

![实验结果](https://github.com/casey-li/MIT6.5840/blob/main/Lab2/Lab2B/result/pic/Lab2B%E7%BB%93%E6%9E%9C.png?raw=true)

通过了 1000 次的连续测试 ( 500 次带日志, 500 次无日志, 无日志的结果在 `Lab2B/result/test_2B_500times.txt`)

---

## :wink: Lab 2C - persistence

### :cherry_blossom: 目标
实现 Raft 的持久化, 让即使基于 Raft 的服务器重启了也能在它发生中断的地方恢复服务。因此每当 Raft 的状态被更改时, 需要将其持久化写入磁盘, 并在重新启动时从磁盘读取状态

### :mag: 提示
- :one: 实现不需要真正写入磁盘, 只需利用 `persister.go` 中提供的 `Persister` 对象进行保存和恢复即可
- :two: 完成 `raft.go` 中的 `persist()` 和 `readPersist()` 函数, 有例子 (仿着写即可), 目前只需将 `nil` 传给 `persister.Save()` 的第二个参数
- :three: 在更改持久化状态的地方加入对 `persist()` 的调用
- :four: Lab 2C 的测试强度比 Lab 2A、2B 都要高很多很多, 出现问题很可能是由 Lab 2A 或 2B 中的 bug 引起的

### :beers: 实现

**图2中指出来了持久化参数为 `Term, VoteFor, Log`**

相比于 2A, 2B 很简单的一节

直接仿照着 `persist()` 和 `readPersist()` 中给的例子对持久化参数进行持久化以及读取即可, 并在它们发生改变的时候调用 `persist()` 即可。很快就可以写好, 但是测试的时候就会发现各种 bug :sob:。 

bug 几乎都是 Lab2A 和 Lab2B 引入的, 一旦测试案例上强度了 (网络不稳定, 服务器断联, 崩溃, 发送的 RPC 得不到响应或者会收到远古时期的 RPC 等等), 这些 bug 就出来了

**在测试 Lab2A Lab2B 的时候不要以为简单跑几次都过了就没啥事了！！！**

发现这些问题后自己又重新回去写了 Lab2A Lab2B, 每个都测了 1000 次, 没问题后继续做的 Lab2C (500 次带日志的用于找 bug, 500 次不带日志的最终结果以及典型的出错日志都上传到 raft/test_result 文件里了)


### :sob: bugs
跑 Lab 2C 的测试案例很艰难, 卡了很久。对着日志找到了不少 Lab 2A 和 Lab 2B 引入的 bug, 这里记录一下

#### 1. Lab 2C 中的 `TestFigure8Unreliable2C`

**`Test (2C): Figure 8 (unreliable)`**

:lollipop: 这个测试案例会不断的提交命令, 休眠一段时间后根据概率断开 leader 的连接, 并在连接的节点数少于一半时根据概率重连一个服务器。最后连接所有服务器, 提交命令, 检查是否能达成共识

跑该测试案例的时候, 会出现最后不能达成共识的情况。检查日志发现问题在于最后选出的 leader 跟其他节点的日志差异过大。 原先 Lab2B 的实现中若 leader 发送心跳时接收到的回复是 false, leader 会减小发送的 `AppendEntriesArgs` 中的 `rf.nextIndex[peerId]`, 但是这种每次仅减小 1 的方案无法在有限的时间内确定跟其他服务器发生冲突的下标位置 (下标大概只能从600减小到400, 然后 fail)

**:thought_balloon: 解决方案**

优化回复的 `AppendEntriesReply` 结构体

```go
type AppendEntriesReply struct {
	Term    int
	Success bool
	// Lab2C 新增的，避免 leader 每次只往前移动一位；若日志很长的话在一段时间内无法达到冲突位置
	ConflictIndex int
	ConflictTerm  int
}
```
此时在 `AppendEntries` 函数中, 若 `reply.Success == false`, 补充这两个字段。leader 收到回复后根据它快速修改 `PrevLogIndex`

```go
func (rf *Raft) AppendEntries(args *AppendEntriesArgs, reply *AppendEntriesReply) {
	// 1 若状态为 Dead, 直接返回

	// 2 若 args.Term 更大, 调用 becomeFollower(args.Term) 更新任期等信息

	// 3 若任期仍不等 (当前任期更大) 回复 false 和当前任期, leader 收到后会变为 follower
	if args.Term == rf.currentTerm {
		if rf.state != Follower {
			// 4 任期相等但是当前不是 follower 的话,调用 becomeFollower(args.Term) 变成 follower
		}
		// 5 更新选举定时器的开始时间
		// [Lab2B] 6.1 检查参数中的上一个日志下标是否小于自己的日志长度并且任期相同, 不满足直接返回 false, 否则之后返回 true
		if args.PrevLogIndex < len(rf.log) && args.PrevLogTerm == rf.log[args.PrevLogIndex].Term {
			// [Lab2B] 6.1.1 从 args.PrevLogIndex + 1 开始找日志不同之处 (不断比较直到下标越界或者日志任期不等)

			// [Lab2B] 6.1.2 若 argsLogIndex < len(args.Entries) 则将参数中的后序日志拼接到 log[:insertIndex] 后面

			// [Lab2B] 6.1.3 若 args.LeaderCommit > rf.commitIndex, 更新 rf.commitIndex 并给 notifyNewCommitChan 发通知
		} else {
			// [Lab2C] 6.2 replay false, 此时填写 ConflictIndex 和 ConflictTerm
			if args.PrevLogIndex >= len(rf.log) {
				reply.ConflictIndex = len(rf.log)
				reply.ConflictTerm = -1
			} else {
				reply.ConflictTerm = rf.log[args.PrevLogIndex].Term

				var ind int
				for ind = args.PrevLogIndex - 1; ind >= 0; ind-- {
					if rf.log[ind].Term != reply.ConflictTerm {
						break
					}
				}
				reply.ConflictIndex = ind + 1
			}
		}
	}
	// [Lab2C] 7 若持久化状态发生了改变, 调用 rf.persist()
}

func (rf *Raft) runHeartBeats() {
	// 1 检查状态, 若不是 leader 的话直接退出 (可能是旧 leader 调用的), 记录当前任期 nowTerm

	// 2 开启多个协程发送 AppendEntries RPC
	// Lab2B 中需要发送 PrevLogIndex, PrevLogTerm, Entries, LeaderCommit
	for peerId := range rf.peers {
		go func(peerId int) {
			for !rf.killed() {
				// 2.1 等待 AppendEntriesReply, 若调用失败直接退出
				
				// [Lab 2C] 若当前非 leader, 退出

				// 2.2 检查 reply.Term, 若比自己的 term 更大, 调用 becomeFollower(reply.Term) 并退出
	
				if rf.state == Leader && currentTerm == reply.Term && !rf.killed() {
					if reply.Success {
						// Lab2B 2.3.1 更新 rf.nextIndex[peerId], rf.matchIndex[peerId]
	
						// Lab2B 2.3.2 保存当前的 commitIndex (savedCommitIndex), 若有半数的节点的 matchIndex[j] 大于 commitIndex, 更新 commitIndex
	
						// Lab2B 2.3.3 若 commitIndex > savedCommitIndex, 给 notifyNewCommitChan 发送通知，提交新的日志条目
					} else {
						// Lab2C 2.4.1 若收到的回复为 false, 更新 rf.nextIndex[peerId], 重发心跳
						if reply.ConflictTerm >= 0 {
							lastIndex := -1
							for i := len(rf.log) - 1; i >= 0; i-- {
								if rf.log[i].Term == reply.ConflictTerm {
									lastIndex = i
									break
								}
							}
							if lastIndex >= 0 {
								rf.nextIndex[peerId] = lastIndex + 1
							} else {
								rf.nextIndex[peerId] = reply.ConflictIndex
							}
						} else {
							rf.nextIndex[peerId] = reply.ConflictIndex
						}
						rf.mu.Unlock()
						continue
					}
				}
			}
		}(peerId)
	}
}

```

#### 2. Lab 2C 中的 `internalChurn`

**`Test (2C): unreliable churn`**

:lollipop: 跑该测试案例的时候偶尔会出现死锁的情况, 研究日志后发现死锁出在 Lab 2B 实现中的 `runHeartBeats()` 和 `commitCommand()` 函数上。原先的实现中, 一旦其他服务器回复心跳为 true 是会加锁, 然后处理后序操作(更新 nextIndex, 当大于半数服务器都收到了日志后还会提交新的日志), 最后解锁。但是因为多个协程之间会抢释放掉的锁, 可能存在下面情况

leader 给所有服务器发心跳, leader 收到了 1 的正确回复时加锁并发现可以提交新日志了, 给管道发送信号, 自己阻塞等待管道取数据。管道取出数据后的一瞬间 (锁释放了并且 `commitCommand()` 函数中的下一步就是加锁, 但是锁被 leader 中的另一个协程拿到了), leader 又收到了 2 的正确回复, 加锁并发现可以提交更多的新数据, 给管道发送数据, 自己阻塞了 (但是因为负责取数据的协程已经进入提交新数据的逻辑中了并且在等待其他协程释放锁, 所以不能取新发来的提交数据的信号)。此时产生死锁！！

```go
func A () {
	for i := range peers {
		go func(i int) {
			// ...
			if succ := rf.sendHeartBeats(peerId, &args, &reply); succ {
				mu.Lock()
				// ...
				if satisfy requirements {
					ch <- struct {}{}
				}
			}
			mu.UnLock()
		}(i)
	}
}

func B() {
	for {
		case <- ch:
		mu.Lock()
		// ...
		mu.UnLock()
	}
}

```

**:thought_balloon: 解决方案**

- :one: 给管道容量, 比如服务器的数目 (最多也只会同时有 `len(peers) - 1` 个协程发送心跳), 让发送可提交新日志的协程不阻塞, 继续执行就会直接释放锁。不会出现提交新日志的协程和发送有新数据可提交的协程抢占锁的情况
- :two: 因为本函数中发送有新数据到来的信号给通道后, 无后序需要加锁的操作, 因此可以直接先释放锁, 再发送信号。此时阻塞等待 `commitCommand()` 函数取数据不会死锁

自己采用的方案2

```go
func A () {
	for i := range peers {
		go func(i int) {
			// ...
			if succ := rf.sendHeartBeats(peerId, &args, &reply); succ {
				mu.Lock()
				// ...
				if satisfy requirements {
					mu.UnLock()
					ch <- struct {}{}
					return
				}
			}
			mu.UnLock()
		}(i)
	}
}

func B() {
	for {
		case <- ch:
		mu.Lock()
		// ...
		mu.UnLock()
	}
}

```

#### 3. data race

**:lollipop: 跑多次 Lab2C 出现的问题, 并非某个具体测试案例检查出来的 bug, 用 -race 测试时出现了 data race 但是检查代码时找不到问题所在**

**:thought_balloon: 解决方案**

很大概率是发送心跳时的参数中的 `Entries` 属性, 不要直接 `Entries: rf.log[nowLogIndex:]`, 拷贝一份 `rf.log[nowLogIndex:]` 再发送拷贝的日志就可以解决 data race 的问题

#### 4. Lab 2B 中的 `TestRPCBytes2B`

**Test (2B): RPC byte count**

:lollipop: 该测试案例用于测试本次运行中发送的 RPC 的总的字节数, 并给了一个理论的上界值, 若真实发送的字节数超过了理论的最大值则会提示 `test_test.go:179: too many RPC bytes; got 222422, expected 150000`

这个bug是跑了很多次后在发现的, 检查日志后发现问题在于可能某次运行中 leader 会存在两个后台协程在发送心跳, 导致发送和接收的 RPC 数目远大于理论最大值

- 该情况发生的 case 如下 (`日志记录在test_result/TestRPCBytes_fail_case.txt`) 

Lab2A 中原先的 `startElection()` 实现为一个节点变成候选者后, 发起请求投票并 go rf.ticker() 开启新的后台定时器以保证若本次选举失败的话开启下一轮选举, 若收到的同意票数大于半数则转变为领导者。原先的 `ticker()` 实现为进来后加锁, 根据状态选择执行选举定时器还是心跳定时器

但是！因为锁的竞争问题, 可能发生如下情况: 另一个协程 g2 进入 `ticker()` 函数后, 在加锁前当前候选者收到了其他节点的回复并上了锁, 此时 g2 阻塞; 但是在释放完锁后, 锁并未被 g2 拿到而是被给其他节点发送投票请求的协程拿到了, 不巧的是这刚好为同意票的第 n/2 + 1 张票, Candidate 转变为了 leader, 并在 `becomeLeader()` 中启动了一个心跳定时器, 一切设置完毕后释放锁。此时锁才被 g2 拿到, 它加锁检查状态后发现为 leader, **又开启了一个心跳定时器**。之后这两个心跳定时器每隔一段时间都会发送心跳, 导致 RPC 字节数过大！！！

**:thought_balloon: 解决方案**

究其原因在于操作并不是一气呵成的, 进入 `ticker()` 函数时当前节点的状态和在 `ticker()` 内拿到锁后的当前节点的状态可能已经发生了改变, 进而执行了非预期的定时器函数！！

自己修改了 `ticker()` 函数的实现, 通过传入当前节点的状态而不是在函数内部重新获取状态来执行相应的定时器。这样 `becomeLeader()` 中启动后台定时器的调用变成了 `go rf.ticker(Follower)`, 而 `becomeLeader()` 中启动后台定时器的调用变成了 `go rf.ticker(Leader)`。这样可以保证启动的定时器跟调用时的状态一致

```go
// 原来的 Lab2A 的ticker(), 依靠加锁后检查当前节点状态选择定时器，可能因为锁的抢占问题导致调用 ticker() 时节点的状态和获取到的状态不同
func (rf *Raft) ticker() {
	if !rf.killed() {
		rf.mu.Lock()
		state := rf.state
		rf.mu.Unlock()
		switch state {
		case Follower:
			rf.runElectionTimer()
		case Candidate:
			rf.runElectionTimer()
		case Leader:
			rf.heartBeatsTimer()
		}
	}
}

// 修改后的，依靠调用时的节点状态直接选择对应的定时器执行
func (rf *Raft) ticker(state RuleState) {
	if !rf.killed() {
		switch state {
		case Follower:
			rf.runElectionTimer()
		case Candidate:
			rf.runElectionTimer()
		case Leader:
			rf.heartBeatsTimer()
		}
	}
}
```

#### 5. Lab 2B 中的 `TestBackup2B`

**Test (2B): leader backs up quickly over incorrect follower logs**

:lollipop: 该测试案例会在选出领导者后断链超过一半的其他机器再发送大量命令 (它们不会被真正提交); 随后断联当前还连接的机器并连接之前断联的机器, 然后再次发送大量命令 (因为机器总数大于一半, 它们会重选出领导者并成功提交这些命令); 之后再断联一台机器并发送大量命令 (不会被真正提交); 连接最开始的未被断联的机器 (包含第一代领导者) 以及刚刚被断联的机器并断联其他所有机器, 发送大量命令 (机器数刚好超过一半, 在选出领导者后会被提交); 最后连接所有机器, 发送一条命令并检查能否达成共识

这个bug是跑了超级多次后在发现的, 检查日志后发现问题在重置选举开始时间 (该问题发生的概率超级超级小, 看日志的时候都觉得很不可思议)。出错并不是在最后出错的, 在之前就出问题了, 迟迟选不出领导者

- 该情况发生的 case 如下 (`日志记录在test_result/TestBackUp_failure_case.txt`) 

Lab2A 中原先的 `becomeFollower()` 实现为修改任期, 状态, 重置投票结果, 重置选举定时器并开启新的当前任期下的后台定时器。`RequestVote()` 中当当前节点收到更大的任期的节点发来的请求投票的 RPC 后会调用 `becomeFollower()`, 这样在上述测试案例中产生了如下问题

假设当前未断联的节点为1, 2, 3 (3中的日志最新, 但是因为之前断联的缘故1, 2的任期疯狂增大); 超时后 1 或 2 发送请求投票, 更新了 3 的任期 (因为 3 的日志最新, 所以永远也不会给 1, 2 投赞成票, 即 1, 2 永远也不可能成为领导者)和选举定时器, 然后自己也开启了新的选举定时器。**但是巧的是每次随机出来的选举超时时间都是 1 或 2 更小一点, 然后它们的任期自增, 请求其他节点投票并让所有节点重置选举定时器 (真正能当领导者的 3 在 7181-8458 这长达 1400 行的日志中每次随机出来的超时时间都大于 1 或 2 :sweat_smile:), 看日志发现这个过程不断重复, 然后因为达不成共识失败**

**:thought_balloon: 解决方案**

其实问题在于 Lab2A 中 `becomeFollower()` 的实现有问题, 并不是所有情况下变成 Follower 都需要重置选举开始时间的。在 `RequestVote()` 中, 只有投赞成票的时候才需要重置选举开始时间, 而发现接收到的参数中的任期更大只需修改任期, 重置投票结果, 修改状态即可。因此将重置选举开始时间这一步从 `becomeFollower()` 中删除即可, 这样当 1 或 2 发起请求投票时, 3 必定投反对票并且不会重置选举开始时间, 最差情况为 1 或 2 依次开始选举, 然后失败, 随后 3 开始选举并当选 leader

#### 6. `failed to reach agreement`

:lollipop: 在多次跑 Lab2B 的过程中可能出现的问题, 不是具体某个测试案例下发现的。

这个bug是多次运行后在发现的, 检查日志后发现问题在于原先 Lab 2A 的 `RequestVote()` 逻辑可能会导致同时存在两个领导者！！

原先的逻辑为当收到的请求投票的参数中的任期更大的话, 会修改任期, 状态, 投票结果并 `go becomeFollower()` (直接修改那些参数是为了不影响后序逻辑, 让其可以正常投票并在投票结束后并行的调用 `becomeFollower()`, 因为有锁)。但是这样存在的一个问题是假设仅有三个节点并且有两个任期相等并且更大的节点几乎同时超时并发起请求投票, 当前节点在回复第一个节点时修改了任期并给它投了赞成票, 随后并行的协程获取了锁并将投票结果又改为了 -1; 然后第二个节点的请求投票信息过来了, 它又投了赞成票, 导致存在两个 leader

***当且仅当两个节点几乎同时变为候选者并且它们的任期相等且比另一个节点更大并且锁的抢占顺序为 `RequestVote(), becomeFollower(), RequestVote()` 才会出现这种情况***

**:thought_balloon: 解决方案**

取消了并行执行 `becomeFollower()`, 让其串行执行, 这样只会执行一次 `voteFor = -1`, 但是因为原先 `becomeFollower()` 中上锁了, 所以可以调用前先解锁, 调用 `becomeFollower()` 后再上锁。但是这么改利用小锁可能会出现锁抢占的问题; 为了简化逻辑, 自己直接去掉了 `becomeFollower()` 中的加锁解锁操作, 因为调用 `becomeFollower()` 时本来就处于锁的掌控范围内, 所以并不会出现资源抢占的问题; 同理, 自己也把 `becomeLeader()` 也改了, 也是去掉了锁, 串行修改并用调用该函数内存在的粗粒度锁来避免调用过程中的资源竞争问题

#### 7. Lab 2C 中的 `TestFigure8Unreliable2C`

**`Test (2C): Figure 8 (unreliable)`**

:lollipop: 这个测试案例会不断的提交命令, 休眠一段时间后根据概率断开 leader 的连接, 并在连接的节点数少于一半时根据概率重连一个服务器。最后连接所有服务器, 提交命令, 检查是否能达成共识

跑了500次其中有3次出错，报错提示为 `apply error: commit index=X1 server=X2 X3 != server=X4 X5` , 意思为在提交命令时测试程序发现 X2 服务器提交的下标为 X1 的日志的命令 X3 与 X4 服务器在下标为 X1 的日志中提交过的命令 X5 不等，发生错误。产生错误的日志记录在了 `test_result/TestFigure8Unreliable2C_failure_case` 

检查日志发现原先 Lab2A 的 `startElection()` 实现中若网络不稳定的话存在问题, 会被很久很久之前发送了但一直未收到响应的 RPC 影响到。案例如下 (这个是极其特殊的情况, 找了快一天的 bug 才找到的 :sob:)

1. term = 31 时选出了 leader 但是并未将收到的命令同步给其余节点就被断联了, 0 在 `term = 32` 时发起选举 (4170行), 可以成为 leader 但是因为网络关系在收到半数赞同票前又超时了, 不断重复直到 term = 35 时赢得了选举 (4465行)
2. 在 0 的领导下, 所有节点都达成了共识, 提交了下标为 0-32 的命令 (4465 ~ 4488 行, term 为 0 ~ 19)
3. 0 被断开了连接, 随后 0 一直在接收新命令并加入自己的日志 (term = 35) 但是无法同步给其他节点。在此期间原 leader 回来了并将 term = 31 的命令同步给了其余节点节点, 然后又被断联
4. 开始重选 leader, 0 回来后知道自己是旧时代的 leader, 重新变为了 follower 但是又被断联了。最巧最巧的是 0 断联后一直在尝试成为新的 leader, 自增 term 等待同意 (4725, 4837 行, 跟外界的 term 同步了), 然后 0 的 term 来到了 38, 此时外界的 term 也恰好为 38。这时 0 在 term = 32 时的一个请求投票的回复到了, 恰好超过了半数, 它成为了 leader (4927行), 外界同样在 term = 38 时选出了一个 leader (5055行)
5. 外界接收 term = 38 的命令并同步, 但是 0 同样在接收 term = 38 时的命令
6. 0 回来后肯定当不了 leader (外界的 term 已经到 63), 然后不断向前回溯 `PrevLogIndex`, 因为 0 的日志更长且全部都是 term = 38 的日志。当前 leader 根据 0 的 RPC 响应很快就回溯到了 term = 38 的日志部分这里, 因为 follower 仅检查相同下标处日志的任期来确定日志是否相等, 所以 0 会覆盖掉了自己的后序日志 (但是前面仍有 term = 35 的日志), 并根据 leader 发来的 commitIndex 对日志进行提交, 然后发生冲突

```go
[1] commits log from 152 to 158
// 正常的 1 的日志，已经跟其他节点同步
[1] log: [[{MIT6.5840 ! 0} {1 1} {4 4} {5 4} {6 4} {7 4} {7 4} {9 4} {9 4} {9 4} {10 4} {10 4} {10 4} {10 4} {10 4} {11 4} {11 4} {12 4} {13 4} {14 4} {20 9} {21 9} {22 9} {23 9} {24 9} {25 9} {26 9} {27 9} {29 17} {33 19} {34 19} {35 19} {36 19} {37 19} {38 19} {39 19} {40 19} {40 19} {41 19} {42 19} {42 19} {43 19} {44 19} {45 19} {47 19} {47 19} {82 31} {83 31} {83 31} {83 31} {84 31} {84 31} {84 31} {84 31} {85 31} {86 31} {87 31} {88 31} {89 31} {89 31} {89 31} {89 31} {89 31} {89 31} {89  31} {89  31} {89  31} {89  31} {89  31} {89  31} {89  31} {89  31} {89  31} {89  31} {89  31} {89  31} {89  31} {89  31} {89  31} {89  31} {89  31} {89  31} {89  31} {90  31} {90  31} {91  31} {92  31} {92  31} {92  31} {93  31} {95  31} {96  31} {96  31} {97  31} {97  31} {97  31} {98  31} {98  31} {98  31} {99  31} {100 31} {101 31} {101 31} {102 38} {103 38} {103 38} {104 38} {105 38} {106 38} {107 38} {108 38} {108 38} {109 38} {110 38} {111 38} {113 38} {113 38} {113 38} {113 38} {114 38} {116 38} {117 38} {118 38} {119 38} {120 38} {121 38} {122 38} {122 38} {122 38} {122 38} {123 38} {128 56} {129 56} {130 56} {131 56} {132 56} {133 56} {134 56} {135 56} {136 56} {137 56} {138 56} {139 56} {140 56} {141 56} {144 63} {145 63} {146 63} {147 63} {148 63} {149 63} {149 63} {149 63} {150 63} {151 63} {152 63} {153 63} {154 63} {155 63}]]
// 此时 0 的日志, 最后全是 term = 38 的日志
[0] log: [[{MIT6.5840 ! 0} {1 1} {4 4} {5 4} {6 4} {7 4} {7 4} {9 4} {9 4} {9 4} {10 4} {10 4} {10 4} {10 4} {10 4} {11 4} {11 4} {12 4} {13 4} {14 4} {20 9} {21 9} {22 9} {23 9} {24 9} {25 9} {26 9} {27 9} {29 17} {33 19} {34 19} {35 19} {36 19} {37 19} {38 19} {39 19} {40 19} {40 19} {41 19} {42 19} {42 19} {43 19} {44 19} {45 19} {47 19} {47 19} {89 35} {90 35} {91 35} {92 35} {92 35} {92 35} {93 35} {94 35} {95 35} {96 35} {97 35} {97 35} {97 35} {98 35} {98 35} {98 35} {98 35} {99 35} {100 35} {101 35} {102 38} {102 38} {102 38} {102 38} {102 38} {102 38} {102 38} {102 38} {102 38} {102 38} {102 38} {102 38} {102 38} {102 38} {102 38} {102 38} {102 38} {102 38} {103 38} {103 38} {104 38} {105 38} {106 38} {107 38} {108 38} {108 38} {109 38} {110 38} {111 38} {112 38} {113 38} {113 38} {113 38} {114 38} {115 38} {116 38} {117 38} {118 38} {119 38} {120 38} {121 38} {122 38} {122 38} {122 38} {123 38} {124 38} {124 38} {125 38} {126 38} {127 38} {127 38} {127 38} {127 38} {127 38} {127 38} {127 38} {127 38} {127 38} {127 38} {127 38} {127 38} {127 38} {127 38} {127 38} {127 38} {127 38} {127 38} {127 38} {127 38} {127 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {128 38} {129 38} {130 38} {131 38} {132 38} {133 38} {134 38} {135 38} {136 38} {137 38} {138 38} {139 38} {140 38} {141 38} {142 38} {143 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {144 38} {145 38} {146 38} {147 38} {148 38} {149 38} {149 38} {149 38} {150 38} {151 38} {152 38} {153 38} {154 38} {155 38} {156 38} {157 38} {158 38} {159 38} {160 38} {161 38} {162 38} {163 38} {164 38} {165 38} {166 38}]]
// 被同步后截去了后面一部分 term = 38 的日志，然后根据 leader 的 commitIndex 向上提交命令时产生了冲突
[0] log: [[{MIT6.5840 ! 0} {1 1} {4 4} {5 4} {6 4} {7 4} {7 4} {9 4} {9 4} {9 4} {10 4} {10 4} {10 4} {10 4} {10 4} {11 4} {11 4} {12 4} {13 4} {14 4} {20 9} {21 9} {22 9} {23 9} {24 9} {25 9} {26 9} {27 9} {29 17} {33 19} {34 19} {35 19} {36 19} {37 19} {38 19} {39 19} {40 19} {40 19} {41 19} {42 19} {42 19} {43 19} {44 19} {45 19} {47 19} {47 19} {89 35} {90 35} {91 35} {92 35} {92 35} {92 35} {93 35} {94 35} {95 35} {96 35} {97 35} {97 35} {97 35} {98 35} {98 35} {98 35} {98 35} {99 35} {100 35} {101 35} {102 38} {102 38} {102 38} {102 38} {102 38} {102 38} {102 38} {102 38} {102 38} {102 38} {102 38} {102 38} {102 38} {102 38} {102 38} {102 38} {102 38} {102 38} {103 38} {103 38} {104 38} {105 38} {106 38} {107 38} {108 38} {108 38} {109 38} {110 38} {111 38} {112 38} {113 38} {113 38} {113 38} {114 38} {115 38} {116 38} {117 38} {118 38} {119 38} {120 38} {121 38} {122 38} {122 38} {122 38} {123 38} {124 38} {124 38} {125 38} {126 38} {127 38} {127 38} {127 38} {127 38} {127 38} {127 38} {127 38} {127 38} {127 38} {127 38} {127 38} {127 38} {127 38} {127 38} {127 38} {127 38} {128 56} {129 56} {130 56} {131 56} {132 56} {133 56} {134 56} {135 56} {136 56} {137 56} {138 56} {139 56} {140 56} {141 56} {144 63} {145 63} {146 63} {147 63} {148 63} {149 63} {149 63} {149 63} {150 63} {151 63} {152 63} {153 63} {154 63} {155 63}]]

2023/07/03 17:40:36 0: log map[1:1 2:4 3:5 4:6 5:7 6:7 7:9 8:9 9:9 10:10 11:10 12:10 13:10 14:10 15:11 16:11 17:12 18:13 19:14 20:20 21:21 22:22 23:23 24:24 25:25 26:26 27:27 28:29 29:33 30:34 31:35 32:36 33:37 34:38 35:39 36:40 37:40 38:41 39:42 40:42 41:43 42:44 43:45 44:47 45:47]; server map[1:1 2:4 3:5 4:6 5:7 6:7 7:9 8:9 9:9 10:10 11:10 12:10 13:10 14:10 15:11 16:11 17:12 18:13 19:14 20:20 21:21 22:22 23:23 24:24 25:25 26:26 27:27 28:29 29:33 30:34 31:35 32:36 33:37 34:38 35:39 36:40 37:40 38:41 39:42 40:42 41:43 42:44 43:45 44:47 45:47 46:82 47:83 48:83 49:83 50:84 51:84 52:84 53:84 54:85 55:86 56:87 57:88 58:89 59:89 60:89 61:89 62:89 63:89 64:89 65:89 66:89 67:89 68:89 69:89 70:89 71:89 72:89 73:89 74:89 75:89 76:89 77:89 78:89 79:89 80:89 81:89 82:89 83:90 84:90 85:91 86:92 87:92 88:92 89:93 90:95 91:96 92:96 93:97 94:97 95:97 96:98 97:98 98:98 99:99 100:100 101:101 102:101 103:102 104:103 105:103 106:104 107:105 108:106 109:107 110:108 111:108 112:109 113:110 114:111 115:113 116:113 117:113 118:113 119:114 120:116 121:117 122:118 123:119 124:120 125:121 126:122 127:122 128:122 129:122 130:123 131:128 132:129 133:130 134:131 135:132 136:133 137:134 138:135 139:136 140:137 141:138 142:139 143:140 144:141 145:144 146:145 147:146 148:147 149:148 150:149 151:149 152:149]
2023/07/03 17:40:36 apply error: commit index=46 server=0 89 != server=4 82
exit status 1
```

**:thought_balloon: 解决方案**

修改 `startElection()` 部分的判断逻辑，来消除过期 RPC 的影响

```go
// 原来的逻辑
func (rf *Raft) startElection() {
	// ...
	for peerId := range rf.peers {
		go func(peerId int) {
			// ...
			if succ := rf.sendRequestVote(peerId, &args, &reply); succ {
				// ...
				if rf.state != Candidate {
					return
				} else if reply.Term > currentTerm {
					rf.becomeFollower(reply.Term)
					return
				} else if reply.Term == currentTerm && reply.VoteGranted {
					// ...
					return
				}
			}
		}(peerId)
	}
	go rf.ticker(Follower)
}

// 改后的逻辑
func (rf *Raft) startElection() {
	// ...
	for peerId := range rf.peers {
		go func(peerId int) {
			// ...
			if succ := rf.sendRequestVote(peerId, &args, &reply); succ {
				// ...
				if rf.state != Candidate {
					return
				} else if reply.Term > currentTerm {
					if reply.Term > rf.currentTerm {
						rf.becomeFollower(reply.Term)
					}
					return
				} else if reply.Term == currentTerm && currentTerm == rf.currentTerm && reply.VoteGranted {
					// ...
					return
				}
			}
		}(peerId)
	}
	go rf.ticker(Follower)
}
```


#### 8. Lab 2C 中的 `TestFigure8Unreliable2C`

**`Test (2C): Figure 8 (unreliable)`**

跑了500次其中有4次报错，报错提示为 `panic: runtime error: index out of range [-1]` , 产生错误的日志记录在了 `test_result/TestFigure8Unreliable2C_failure_case_2` 

检查日志发现错误原因类似于 bug 7, bug 7 产生后自己只是单纯的将原先使用的 currentTerm 都改为了 rf.currentTerm。 但是这样简单的修改 `runHeartBeats()` 在网络不稳定的话存在问题, 会被很久很久之前发送但是刚接收到的 `AppendEntriesReply` 影响到

日志中的特殊情况为 4 在 term = 49 时成为了 leader, 断连回来后又给其它节点发送了心跳, 此时外界的 term 已经到了 85。 4 收到其它节点的回复后知道自己过期了，转变为了 follower，然后再 term = 86 时又当选了 leader，其它节点的 term 随后也更新到了 86。巧的是此时节点 0 收到了 term = 49 时的心跳并回复 `[0] reply AppendEntries [&{Term:86 Success:false ConflictIndex:0 ConflictTerm:0}] to 4` (9406 ~ 9407行)。因为请求参数中的任期跟自己的不等，所以直接回复了 false，无需填充 `ConflictIndex` 和 `ConflictTerm`。4 收到回复后检查条件发现 `rf.state == Leader && rf.currentTerm == reply.Term && !rf.killed()` 都满足, 进而认为这是一个有效的回复, 但是因为 `reply.Success = false && ConflictTerm = 0 && ConflictIndex = 0`, `rf.nextIndex[peerId]` 被更新为了 0。下一次发送心跳时 `PrevLogIndex = -1`，读取日志时报错！！

**:thought_balloon: 解决方案**

修改 `runHeartBeats()` 部分的判断逻辑

```go
// 原来的逻辑
func (rf *Raft) runHeartBeats() {
	// ...
	for peerId := range rf.peers {
		go func(peerId int) {
			for !rf.killed() {
				// ...
				if succ := rf.sendHeartBeats(peerId, &args, &reply); succ {
					// ...
					if reply.Term > rf.currentTerm {
						rf.becomeFollower(reply.Term)
						rf.electionStartTime = time.Now()
						rf.mu.Unlock()
						return
					}

					if rf.state == Leader && rf.currentTerm == reply.Term && !rf.killed() {
						if reply.Success {
							// ...
						} else {
							// ...
						}
					}
				}
				return
			}
		}(peerId)
	}
}

// 改后的逻辑
func (rf *Raft) runHeartBeats() {
	// ...
	for peerId := range rf.peers {
		go func(peerId int) {
			for !rf.killed() {
				// ...
				if succ := rf.sendHeartBeats(peerId, &args, &reply); succ {
					// ...
					if reply.Term > currentTerm {
						if reply.Term > rf.currentTerm {
							rf.becomeFollower(reply.Term)
							rf.electionStartTime = time.Now()
						}
						rf.mu.Unlock()
						return
					}

					if rf.state == Leader && currentTerm == reply.Term && currentTerm == rf.currentTerm && !rf.killed() {
						if reply.Success {
							// ...
						} else {
							// ...
						}
					}
				}
				return
			}
		}(peerId)
	}
}

```
### :rainbow: 结果

![2C结果](https://github.com/casey-li/MIT6.5840/blob/main/Lab2/Lab2C/result/pic/Lab2C%E7%BB%93%E6%9E%9C.png?raw=true)

通过了 1000 次的连续测试 ( 500 次带日志, 500 次无日志, 无日志的结果在 `Lab2C/result/test_2C_500times.txt`)

---

## :wink: Lab 2D - log compaction

### :cherry_blossom: 目标

实现日志压缩功能, 即 snapshot 快照; 因为若不对日志进行压缩的话, 随着时间推移每台服务器上都会保留特别长的日志, 即占用存储空间又不利于服务器崩溃后的快速恢复

### :mag: 提示
- :one: 修改代码让其可以仅存储从某个索引 X 开始的日志部分, 最初可以将 X 设为 0 并在 2B, 2C 上进行测试。然后让 `Snapshot(index)` 丢弃 index 之前的日志并让 X = index, 若一切顺利就可以通过第一个测试
- :two: 修改索引访问逻辑, 因为存在丢弃的日志, 所以此时日志的真正索引已经不等于它在切片中的索引了
- :three: 若 leader 发送心跳时发现应给当前 follower 发送的起始日志被删除了的话, 发送 `InstallSnapshot RPC`, 让 follower 接受并提交快照
- :four: 在单个 `InstallSnapshot RPC` 中发送整个快照, 不要实现图 13 的偏移机制来分割快照
- :five: Raft 必须正确地丢弃旧日志条目 (无引用或指针指向被丢弃的日志条目) 以允许 Go 的垃圾回收机制对内存进行重利用
- :six: 即使日志被修剪，你的实现仍然需要在调用 `AppendEntries RPC` 时发送正确的参数 (上一个日志的任期和索引), 这可能需要保存最新快照的 `lastIncludedTerm` 和 `lastIncludedIndex`（考虑是否应该对其进行持久化处理）

**图 13**

![](https://github.com/casey-li/MIT6.5840/blob/main/Lab2/Lab2D/result/pic/Raft2D-snapshot.png?raw=true)

因为不需要实现偏移机制来分割快照, 所以无需 offset 和 done 这两个属性

### :pizza: 数据结构

新增的数据结构为 `InstallSnapshotArgs` 和 `InstallSnapshotReply`, 用于实现 `InstallSnapshot RPC`, 自己直接将快照命名为了 snapshot, 即图13中的 data

```go
type InstallSnapshotArgs struct {
	Term              int
	LeaderId          int
	LastIncludedIndex int    // 快照中最后一个条目包含的索引
	LastIncludedTerm  int    // 快照中最后一个条目包含的任期
	Snapshot          []byte //快照
}

type InstallSnapshotReply struct {
	Term int
}
```

此外，为了获取日志的真实下标，在原来的 LogEntry 结构体中新增了 Index 属性。那么一个日志在当前切片中的逻辑位置即为 `index - lastIncludedIndex`。

本来打算将 `lastIncludedIndex` 和 `lastIncludedTerm` 保存到 raft 结构体中的，但是这样操作的话持久化时也需要把它们一并处理

随后在 Raft 初始化过程中突然意识到自己并未利用好第一个日志的信息 (之前的 Lab 2B 要求下标从 1 开始, 所以初始化时加入了一个无意义的日志作为了第一个日志)，第一个日志的 `Term` 和 `Index` 刚好可以用来保存 `lastIncludedIndex` 和 `lastIncludedTerm` ！！此外，持久化就会保存日志，因此它们也会被直接持久化

```go
type LogEntry struct {
	Command interface{}
	Term    int
	Index   int
}
```

### :beers: 实现

#### :cherries:  索引转换

为了让所有代码不受被删除的日志的影响而继续使用原下标进行处理，封装了以下函数

```go
// 返回第一个日志的下标, 即 lastIncludedIndex
func (rf *Raft) getFirstIndex() int {
	return rf.log[0].Index
}

// 返回第一个日志的任期, 即 lastIncludedTerm
func (rf *Raft) getFirstTerm() int {
	return rf.log[0].Term
}

// 返回最后一个日志的下标
func (rf *Raft) getLastIndex() int {
	return rf.log[len(rf.log)-1].Index
}

// 返回最后一个日志的任期
func (rf *Raft) getLastTerm() int {
	return rf.log[len(rf.log)-1].Term
}

// 返回最后一个日志的下一个下标
func (rf *Raft) getNextIndex() int {
	return rf.getLastIndex() + 1
}

// 返回下标为 index 处的日志的任期 (原始下标)
func (rf *Raft) getTerm(index int) int {
	return rf.log[index-rf.getFirstIndex()].Term
}

// 这样获取真实的第 index 个日志的调用如下, 没有设计 getLog(index int) LogEntry 是不想多次拷贝一个日志
rf.log[index - rf.getFirstIndex]
```

#### :cherries:  InstallSnapshot RPC

服务器需要检查是否为过期的快照 (比较自己的 `commitIndex` 和 `args.LastIncludedIndex`); 若为满足要求的快照，对自己的日志进行裁剪 (注意至少要预留一个日志大小的空间来存放快照信息), 随后进行持久化处理, 更新自己的信息并将快照发给 `applyChan`

```go
func (rf *Raft) InstallSnapshot(args *InstallSnapshotArgs, reply *InstallSnapshotReply) {

	// 1 若状态为 Dead, 直接返回

	// 2 若 args.Term 更大, 调用 becomeFollower(args.Term) 更新任期等信息并重置选举定时器开始时间
	if args.Term > rf.currentTerm {
		//...
	}
	reply.Term = rf.currentTerm

	// 3 若任期仍不等 (当前任期更大) 回复当前任期, leader 收到后会变为 follower

	if args.Term == rf.currentTerm {
		if rf.state != Follower {
			// 若当前不是 follower 的话, 变成 follower
		}
		// 4.1 更新选举定时器的开始时间，检查是否为过期的快照
		if rf.commitIndex >= args.LastIncludedIndex {
			return
		}

		// 4.2 裁剪日志，设置 rf.log[0] 的相关信息，即 args.LastIncludedTerm 和 args.LastIncludedIndex

		// 4.3 持久化处理，更新 rf.lastApplied, rf.commitIndex，给 rf.applyChan 发送快照
	}
}

func (rf *Raft) sendInstallSnapshot(server int, args *InstallSnapshotArgs, reply *InstallSnapshotReply) bool {
	ok := rf.peers[server].Call("Raft.InstallSnapshot", args, reply)
	return ok
}
```

#### :cherries: Snapshot()

需要先判断一下是否已经生成过快照了，没有的话就删除日志，调用 `persister.Save()` 进行持久化，因为跟 `persist()` 存在大量重复代码，所以声明了一个新函数 `encodeState()` 生成第一个参数

```go
func (rf *Raft) encodeState() []byte {
	w := new(bytes.Buffer)
	e := labgob.NewEncoder(w)
	e.Encode(rf.currentTerm)
	e.Encode(rf.voteFor)
	e.Encode(rf.log)
	raftstate := w.Bytes()
	return raftstate
}

func (rf *Raft) Snapshot(index int, snapshot []byte) {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	lastIndex := rf.getFirstIndex()
	if lastIndex >= index {
		return
	}
	// 第 0 个日志存快照信息, lastIncludedIndex 和 lastIncludeTerm 就是下标为 index 的日志的信息, 因此裁剪时保留它充当快照信息
	var tmp []LogEntry
	rf.log = append(tmp, rf.log[index-lastIndex:]...)
	rf.log[0].Command = nil
	rf.persister.Save(rf.encodeState(), snapshot)
}

func (rf *Raft) persist() {
	rf.persister.Save(rf.encodeState(), rf.persister.ReadSnapshot())
}
```

#### :cherries: runHeartBeats()

当 leader 发现给当前 follower 发送的第一条需要同步的日志被删除后就会调用 `InstallSnapshot RPC`，否则调用 `AppendEntries RPC`

之前都是发现 bug 了以后在 `runHeartBeats()` 里增加判断逻辑, 这次再新增快照部分后这个函数代码太长了, 在测试 2D 的时候对着日志找 bug 也不方便, 因此进行了重写, 拆分成了多个函数并修改了部分逻辑。现在 `runHeartBeats()` 中仅涉及 RPC 参数的设置以及 RPC 的调用, 收到回复后分别由 `handleInstallSnapshotRPCResponse()` 和 `handleAppendEntriesRPCResponse()` 进行处理

之前处理 RPC 的代码在缝缝补补后又臭又长:sob:，都是先进行了一堆判断，否决掉很多种非法情况后再进行处理，但其实只对合法回复进行处理即可（必须满足1、当前节点仍为leader; 2、当且 leader 的任期等于发送 RPC 时参数种的任期），其它情况统一忽略掉就好了

还有一个注意事项为收到正确的回复后，它虽然什么都正确但是可能是上次发的或者是上上次发送的 RPC，为了避免 matchIndex[] 发生回退，每次更新时应取最大值 (之前未发现这个问题，虽然回退后下一次收到响应又会被改过来但是逻辑上还是存在问题，毕竟不应该允许回退现象的发生)，完整代码如下

```go
func (rf *Raft) runHeartBeats() {
	if rf.state != Leader {
		return
	}
	currentTerm := rf.currentTerm
	for peerId := range rf.peers {
		if peerId == rf.me {
			continue
		}
		go func(peerId int) {
			for !rf.killed() {
				rf.mu.Lock()
				if rf.state != Leader {
					rf.mu.Unlock()
					return
				}
				// Lab2D, 有快照要求, 所以可能要发送的日志已经被删除了 (发送快照), 否则仍按 Lab2B 的流程走就行
				firstIndex := rf.getFirstIndex()
				if rf.nextIndex[peerId] <= firstIndex {
					args := InstallSnapshotArgs{
						Term:              currentTerm,
						LeaderId:          rf.me,
						LastIncludedIndex: firstIndex,
						LastIncludedTerm:  rf.getFirstTerm(),
						Snapshot:          rf.persister.ReadSnapshot(),
					}
					rf.mu.Unlock()
					var reply InstallSnapshotReply
					if rf.sendInstallSnapshot(peerId, &args, &reply) {
						rf.mu.Lock()
						rf.handleInstallSnapshotRPCResponse(peerId, &args, &reply)
						rf.mu.Unlock()
					}
					return
				} else {
					// 原先的 Lab2B 的流程
					prevLogIndex, nowLogIndex := rf.nextIndex[peerId]-1, rf.nextIndex[peerId]
					entries := make([]LogEntry, rf.getNextIndex()-nowLogIndex)
					copy(entries, rf.log[nowLogIndex-rf.getFirstIndex():])
					args := AppendEntriesArgs{
						Term:         currentTerm,
						LeaderId:     rf.me,
						PrevLogIndex: prevLogIndex,
						PrevLogTerm:  rf.getTerm(prevLogIndex),
						Entries:      entries,
						LeaderCommit: rf.commitIndex,
					}
					rf.mu.Unlock()
					var reply AppendEntriesReply
					if rf.sendHeartBeats(peerId, &args, &reply) {
						rf.mu.Lock()
						// 可能需要重发
						if rf.handleAppendEntriesRPCResponse(peerId, &args, &reply) {
							rf.mu.Unlock()
							continue
						}
						rf.mu.Unlock()
					}
					return
				}
			}
		}(peerId)
	}
}

func (rf *Raft) handleInstallSnapshotRPCResponse(peerId int, args *InstallSnapshotArgs, reply *InstallSnapshotReply) {
	// 判断回复的合法性，必须满足 1. 当前节点仍是 Leader 2. 当前任期仍等于发送 RPC 时的任期
	if rf.state == Leader && rf.currentTerm == args.Term {
		if reply.Term == rf.currentTerm {
			rf.matchIndex[peerId] = max(rf.matchIndex[peerId], args.LastIncludedIndex)
			rf.nextIndex[peerId] = rf.matchIndex[peerId] + 1
		} else if reply.Term > rf.currentTerm {
			rf.becomeFollower(reply.Term)
			rf.electionStartTime = time.Now()
		}
	}
}

// 处理 AppendEntriesReply, 并决定是否需要继续发送 (当且仅当收到的reply.Success == false)
func (rf *Raft) handleAppendEntriesRPCResponse(peerId int, args *AppendEntriesArgs, reply *AppendEntriesReply) bool {
	// 判断回复的合法性，必须满足 1. 当前节点仍是 Leader 2. 当前任期仍等于发送 RPC 时的任期
	if rf.state == Leader && rf.currentTerm == args.Term {
		if reply.Term == rf.currentTerm {
			if reply.Success {
				rf.matchIndex[peerId] = max(rf.matchIndex[peerId], args.PrevLogIndex+len(args.Entries))
				rf.nextIndex[peerId] = rf.matchIndex[peerId] + 1
				// 统计投票结果, 更新 commitIndex
				savedCommitIndex := rf.commitIndex
				for i := rf.commitIndex + 1; i < rf.getNextIndex(); i++ {
					if rf.getTerm(i) == rf.currentTerm {
						count := 1
						for j := range rf.peers {
							if j != rf.me && rf.matchIndex[j] >= i {
								count++
							}
						}
						if count*2 >= len(rf.peers)+1 {
							rf.commitIndex = i
						} else {
							break
						}
					}
				}
				if rf.commitIndex != savedCommitIndex {
					rf.commitCond.Signal()
				}
			} else {
				// 根据返回的 ConflictTerm 以及 ConflictIndex 快速修正 rf.nextIndex[peerId]
				if reply.ConflictTerm > 0 {
					lastIndex := -1
					firstIndex := rf.getFirstIndex()
					for i := args.PrevLogIndex - 1; i >= firstIndex; i-- {
						if rf.getTerm(i) == reply.ConflictTerm {
							lastIndex = i
							break
						} else if rf.getTerm(i) < reply.ConflictTerm {
							break
						}
					}
					if lastIndex > 0 {
						rf.nextIndex[peerId] = lastIndex + 1
					} else {
						rf.nextIndex[peerId] = max(reply.ConflictIndex, rf.matchIndex[peerId]+1)
					}
				} else {
					rf.nextIndex[peerId] = max(reply.ConflictIndex, rf.matchIndex[peerId]+1)
				}
				return true
			}
		} else if reply.Term > rf.currentTerm {
			rf.becomeFollower(reply.Term)
			rf.electionStartTime = time.Now()
		}
	}
	return false
}
```

### :cherries: 容易出错的地方

相比于 Lab2C 的诸多 bug, 2D 出现的问题还好，基本是因为 2D 的下标逻辑改了以后但是忘记修改原来的代码出现的问题

#### 下标越界，负数下标

注意节点初始化以及成为 leader 后需要修改 `nextIndex[i]`。原先是让它等于日志长度，但是因为现在日志有修剪，只要日志长度小于 `lastIncludedIndex`, 就会在 `runHeartBeat()` 拷贝日志时减出负数下标

应该让其等于最后一个日志对应的 index 的下一个。此外，初始化的时候 `commitIndex` 以及 `lastApplied` 也不能再等于 0 了

```go
func Make(peers []*labrpc.ClientEnd, me int,
	persister *Persister, applyCh chan ApplyMsg) *Raft {
	rf := &Raft{}
	rf.peers, rf.persister, rf.me = peers, persister, me
	rf.currentTerm, rf.voteFor, rf.state, rf.dead = 0, -1, Follower, 0
	rf.electionStartTime = time.Now()
	// 设置第一个为空条目, 表示 lastIncludedIndex 和 lastIncludedTerm, 初始化都为 0
	rf.log = make([]LogEntry, 1)
	rf.readPersist(persister.ReadRaftState())
	// 读取的第一个日志记录了之前的 lastIncludedIndex 信息, 若节点崩溃应将 commitIndex 和 lastApplied 设为它
	// 同理 rf.nextIndex[] 和 rf.matchIndex[] 也应做相应的修改
	rf.commitIndex, rf.lastApplied = rf.getFirstIndex(), rf.getFirstIndex()
	rf.nextIndex, rf.matchIndex = make([]int, len(peers)), make([]int, len(peers))
	for i := range rf.nextIndex {
		rf.nextIndex[i], rf.matchIndex[i] = rf.getNextIndex(), 0
	}
	rf.applyChan = applyCh
	rf.commitCond = sync.NewCond(&rf.mu)
	go rf.ticker(Follower)
	go rf.commitCommand()
	return rf
}
```

#### 锁抢占的问题

因为自己之前的实现中每个函数内部都是自己控制锁，然后若在函数 A 中调用函数 B 就会先释放锁再调用 B, B 中再加锁, 但是此时锁很可能被其它协程拿走了，然后 B 拿到锁的时候自身状态发生了改变，就容易出现问题

建议只在少量函数中使用锁，比如 A，其它函数如 B 就在 A 持有锁的时候调用就可以了，这样 B 就不用加锁，并且能保证数据的一致性

自己改了 `heartBeatsTimer(), runHeartBeats(), handleInstallSnapshotRPCResponse(), handleAppendEntriesRPCResponse(), commitCommand` 这一套函数的处理逻辑以及 `runElectionTimer(), startElection(), becomeFollower(), becomeLeader()` 这一套处理逻辑，简化了锁的处理逻辑

此外，提交命令部分取消了原先的通过给管道发送信息来提交命令的逻辑，因为这样写必须得先释放锁然后等待提交函数取出管道数据并上锁后才能进行命令的提交，为了简化锁的处理逻辑，换成了条件变量。在需要提交新命令的地方 `Signal()` 唤醒提交函数即可，注意最后的 lastApplied 同样需要取大以避免回退

```go
func (rf *Raft) commitCommand() {
	for !rf.killed() {
		rf.mu.Lock()
		for rf.lastApplied >= rf.commitIndex {
			rf.commitCond.Wait()
		}
		logEntries := make([]LogEntry, rf.commitIndex-rf.lastApplied)
		firstIndex, commitindex := rf.getFirstIndex(), rf.commitIndex
		copy(logEntries, rf.log[rf.lastApplied+1-firstIndex:rf.commitIndex+1-firstIndex])
		rf.mu.Unlock()
		for _, entry := range logEntries {
			rf.applyChan <- ApplyMsg{
				CommandValid: true,
				Command:      entry.Command,
				CommandIndex: entry.Index,
			}
		}
		rf.mu.Lock()
		rf.lastApplied = max(rf.lastApplied, commitindex)
		rf.mu.Unlock()
	}
}
```

#### 重置选举定时器开始时间

在所有跟 leader 相关的 RPC 函数中 (`AppendEnteries RPC, InstallSnapshot RPC`)，当当前节点收到的参数或者回复中的任期更大时，当前节点都应该转变为 follower 并且重置选举开始时间；只有在 `RequestVote RPC` 中收到了任期更大的参数时需要检查自身状态，若当前状态本来就是 follower 的话，不更新选举定时器的开始时间，避免一个不能当选 leader 的节点超时后不断更新其它节点的开始时间，然后一直选不出 leader


### :rainbow: 结果

![2D_1](https://github.com/casey-li/MIT6.5840/blob/main/Lab2/Lab2D/result/pic/Lab2D%E7%BB%93%E6%9E%9C.png?raw=true)

2D 可以同时跑多个进行测试，不然太慢了，跑一次几乎 270s 左右
通过了 500 次的压力测试，实验结果记录在 `Lab2D/result/test_2D_500times.txt`

## Lab2 最终结果



通过了 500 次的压力测试，实验结果记录在 `Lab2/result/test_2_500times.txt`

---
# :rose: 参考

:one: [有关 Raft 工作流程的动画网址](http://thesecretlivesofdata.com/raft/#home)，有助于快速理解 Raft

:two: 下面的博客分四部分介绍了 Raft 的实现，讲的很好 ！！！

:thought_balloon: [***Part 0 - Introduction***](https://eli.thegreenplace.net/2020/implementing-raft-part-0-introduction/)

:thought_balloon: [***Part 1 - Elections***](https://eli.thegreenplace.net/2020/implementing-raft-part-1-elections/)

讲解了状态之间的转移（follower、leader 和 candidate），RPC请求（RequestVotes、AppendEntries）和响应, 注意本部分并未涉及日志的相关内容

:thought_balloon: [***Part 2 - Commands and Log Replication***](https://eli.thegreenplace.net/2020/implementing-raft-part-2-commands-and-log-replication/)

主要讲解当一个客户给 leader 发送命令后，leader 如何处理并通知 follower 复制日志；以及 follow 收到 leader 的 AE 请求后，如何处理

:thought_balloon: [***Part 3 - Persistence and Optimizations***](https://eli.thegreenplace.net/2020/implementing-raft-part-3-persistence-and-optimizations/)

讲解了如何做持久化处理，处理哪些内容以及应在哪些地方做相应的修改。此外讲解了索引的优化，即在发生冲突后如何快速定位下一次发送的日志

:three: [别人写的所有 Lab 的一个总结](https://github.com/SwordHarry/MIT6.824_2021_note/tree/main)

可以在做实验前看一下相应的实验部分，同样列举了一些常见的错误，有助于少走弯路！！