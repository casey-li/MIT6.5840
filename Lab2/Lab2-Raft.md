# Lab 2、Raft


Lab2 系列为 Raft 分布式一致性协议算法的实现，Raft 将分布式一致性共识分解为若干个子问题
- leader election，领导选举 (Lab 2A)
- log replication，日志复制 (Lab 2B)
- safety，安全性(Lab 2B & 2C)；2C 除了持久化还有错误日志处理

以上为 raft 的核心特性，除此之外，要用于生产环境，还有许多地方可以优化
- log compaction，日志压缩-快照(lab2D)
- Cluster membership changes，集群成员变更

## Lab 2A - leader election

**:cherry_blossom: 目标**：实现 Raft 的 leader election 和 heartbeats, 注意是没有日志条目的 `AppendEntries` RPC

**:cherry_blossom: 效果**：选出一个单一的领导者，如果没有瘫痪，领导者继续担任领导者，如果旧领导者瘫痪或旧领导者的数据包丢失，则由新领导者接管丢失

**要求中反复提及注意图2**

![Fig 2](https://github.com/SwordHarry/MIT6.824_2021_note/raw/main/lab/img/008i3skNgy1gvajftq7jmj60u00xyk1402.png)

> 注意实验提示中说明了测试器将心跳限制为了每秒 10 次，并且要求在旧领导失败后的 5s 内必须选出新的领导者，因此**必须使用比论文中 150 - 300 ms 更大的选举超时时间**，但也不能太大

### 各个角色的职责

每个角色 (leader, follower 和 candidate) 都有一个后台的定时器, 本实验中 leader 的定时器设为了 100ms, follower 和 candidate 设为了 [250, 400] 之间的随机数

- :one: follower 和 leader 的定时器结束后会检查条件 (因为在该期间内所有节点的状态可能已经发生了改变), 若一切正常则可以开始选举, 发送请求投票, 根据响应做下一步处理
- :two: leader 的定时器结束后同样需要检查它自己的状态, 若仍为 leader就可以给其它所有节点发送心跳, 等待响应,根据响应做下一步处理

RPC 请求

- :one: leader 负责周期性地广播发送 `AppendEntries` RPC 请求
- :two: candidate 负责周期性地广播发送 `RequestVote` RPC 请求
- :three: follower 仅负责被动地接收 RPC 请求，从不主动发起请求

`AppendEntries` RPC, 用于**发送心跳**和**复制日志 (Lab2A 暂时不用管)**

`RequestVote` RPC, 用于候选人拉票

### 数据结构
看 Fig.2 即可，给的很清楚了
```go

type Raft struct {
	mu        sync.Mutex          // Lock to protect shared access to this peer's state
	peers     []*labrpc.ClientEnd // RPC end points of all peers
	persister *Persister          // Object to hold this peer's persisted state
	me        int                 // this peer's index into peers[]
	dead      int32               // set by Kill()

	// Your data here (2A, 2B, 2C).
	// Look at the paper's Figure 2 for a description of what
	// state a Raft server must maintain.
	currentTerm int
	voteFor     int
	state       RuleState
	// 下面的 Lab2A 暂且用不到
	log         []LogEntry
	commitIndex int
	lastApplied int
	nextIndex   []int
	matchIndex  []int
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
	PrevLogIndex int
	PrevLogTerm  int
	Entries      []LogEntry
	LeaderCommit int
}

type AppendEntriesReply struct {
	Term    int
	Success bool
}

// RequestVote RPC 的参数和回复结构体
type RequestVoteArgs struct {
	Term         int
	CandidateId  int
	LastLogIndex int
	LastLogTerm  int
}

type RequestVoteReply struct {
	Term        int
	VoteGranted bool
}
```

### 实现

**:warning: 注意锁!!! 因为死锁找了很久的 Bug**

**:thought_balloon: 多打日志看各个节点的信息**

**:pencil: 主要函数** 

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

func (rf *Raft) runElectionTimer() {
	// 1 设置超时时间, 记录当前节点的任期 nowTerm
	
	// 2 利用 time.NewTicker() 不断循环检查, 自己设置的每 10 ms 检查一次
	for {

		// 2.1 若变成了 leader, 直接返回
		
		// 2.2 若 nowTerm != rf.currentTerm, 表明这是上一个任期的定时器,直接返回

		// 2.3 检查是否超时, 若超时了则转到 startElection() 开始选举
	}
}

// 进行选举
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
// go rf.ticker() 运行新的选举定时器
func (rf *Raft) becomeFollower(term int) {
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

**:pencil: RequestVote RPC 和 AppendEntries RPC** 

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

## Lab 2B - Log

日志复制

**:cherry_blossom: 目标**：实现 leader 和 follower 的相关代码以实现日志追加, 完善请求投票中投票的判断

Lab2A 中并未完整使用 Fig.2 中给出的所有字段, Lab2B 将进行进一步的完善
### 数据结构

同 Lab2A, 即 Fig.2 中给出的完整字段

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

	// Lab2A 每用到的, 本 Lab 需要用到的字段
	log         []LogEntry // 每个节点的日志
	commitIndex int     // 已知要提交的
	lastApplied int     // 已经应用到状态机上的
	nextIndex   []int   // 仅 leader 使用, 保存要发送给其它节点的下一个日志下标
	matchIndex  []int   // 仅 leader 使用, 保存其它服务器上已经提交的日志下标

	electionStartTime time.Time
}

type AppendEntriesArgs struct {
	Term         int
	LeaderId     int
	PrevLogIndex int        // leader 中保存的 nextIndex[peerID] - 1
	PrevLogTerm  int        // leader 中保存的 log[PrevLogIndex].Term
	Entries      []LogEntry // 发送的日志
	LeaderCommit int        // 当前 leader 的 commitIndex
}

// RequestVote RPC 的参数和回复结构体
type RequestVoteArgs struct {
	Term         int
	CandidateId  int
	LastLogIndex int // candidate 的 len(log) - 1
	LastLogTerm  int // candidate 的 log[LastLogIndex].Term
}
```
Raft 中的 `nextIndex` 为乐观估计, 指代 leader 保留的对应 follower 的下一个需要传输的日志条目
Raft 中的 `matchIndex` 为悲观估计, 指代 leader 已经传输给对应 follower 的日志条目下标，即 follower 目前所拥有的的总日志条目

`commitIndex, lastApplied, nextIndex[], matchIndex[]` 共同组成了 leader 的提交规则. leader 总是最先提交的, 可以认为 leader 为这个集群的代表, leader 提交后, follower 才会提交

> :lollipop: 提交新命令需要多少次 RPC 往返？

:thought_balloon: 两个。第一轮领导者将下一个日志条目发送给追随者，并让追随者确认它们。当领导者处理对 AE 的回复时，它可以根据响应更新其提交索引。第二轮将向关注者发送更新的提交索引，然后关注者将这些条目标记为已提交并将它们发送到提交通道。









---
# :rose: 参考

[有关 Raft 工作流程的动画网址](http://thesecretlivesofdata.com/raft/#home)，有助于快速理解 Raft

下面的博客分四部分介绍了 Raft 的实现，讲的很好 ！！！

:cat: [Part 0 - Introduction](https://eli.thegreenplace.net/2020/implementing-raft-part-0-introduction/)
:rabbit: [Part 1 - Elections](https://eli.thegreenplace.net/2020/implementing-raft-part-1-elections/)，讲解了状态之间的转移（follower、leader 和 candidate），RPC请求（RequestVotes、AppendEntries）和响应, 注意本部分并未涉及日志的相关内容
:wolf: [Part 2 - Commands and Log Replication](https://eli.thegreenplace.net/2020/implementing-raft-part-2-commands-and-log-replication/), 主要讲解当一个客户给 leader 发送命令后，leader 如何处理并通知 follower 复制日志；以及 follow 收到 leader 的 AE 请求后，如何处理
:snake: [Part 3 - Persistence and Optimizations](https://eli.thegreenplace.net/2020/implementing-raft-part-3-persistence-and-optimizations/)
