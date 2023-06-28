# :two_hearts: Lab 2、Raft

[实验介绍](https://pdos.csail.mit.edu/6.824/labs/lab-raft.html)

Lab2 系列为 Raft 分布式一致性协议算法的实现，Raft 将分布式一致性共识分解为若干个子问题
- leader election，领导选举 (Lab 2A)
- log replication，日志复制 (Lab 2B)
- safety，安全性(Lab 2B & 2C)；2C 除了持久化还有错误日志处理

以上为 raft 的核心特性，除此之外，要用于生产环境，还有许多地方可以优化
- log compaction，日志压缩-快照(lab2D)
- Cluster membership changes，集群成员变更

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

![Fig 2](https://github.com/SwordHarry/MIT6.824_2021_note/raw/main/lab/img/008i3skNgy1gvajftq7jmj60u00xyk1402.png)


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
			
			// 2.1 等待 AppendEntriesReply, 若调用失败直接退出

			// 2.2 检查 reply.Term, 若比自己的 term 更大, 调用 becomeFollower(reply.Term) 并退出

			if rf.state == Leader && currentTerm == reply.Term {
				if reply.Success {
					// Lab2B 2.3.1 更新 rf.nextIndex[peerId], rf.matchIndex[peerId]

					// Lab2B 2.3.2 保存当前的 commitIndex (savedCommitIndex), 若有半数的节点的 matchIndex[j] 大于 commitIndex, 更新 commitIndex

					// Lab2B 2.3.3 若 commitIndex > savedCommitIndex, 给 notifyNewCommitChan 发送通知，提交新的日志条目
				} else {
					// Lab2B 2.3.4 nextIndex[peerId] -= 1
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
