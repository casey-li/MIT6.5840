package raft

// this is an outline of the API that raft must expose to
// the service (or tester). see comments below for
// each of these functions for more details.
//
// rf = Make(...)
//   create a new Raft server.
// rf.Start(command interface{}) (index, term, isleader)
//   start agreement on a new log entry
// rf.GetState() (term, isLeader)
//   ask a Raft for its current term, and whether it thinks it is leader
// ApplyMsg
//   each time a new entry is committed to the log, each Raft peer
//   should send an ApplyMsg to the service (or tester)
//   in the same server.

import (
	"bytes"

	"math/rand"

	// "rand"
	"sync"
	"sync/atomic"
	"time"

	"fmt"

	"6.5840/labgob"

	"6.5840/labrpc"
)

// 是否打印日志
const LogOption = false

// var loger *log.Logger

// func init() {
// 	if !LogOption {
// 		return
// 	}
// 	_, err := os.Stat("./log")
// 	if os.IsNotExist(err) {
// 		if err = os.Mkdir("./log", 0775); err != nil {
// 			log.Fatalf("create directory failed!")
// 		}
// 	}
// 	file := "./log/" + time.Now().Format("0102_1504") + ".txt"
// 	f, err := os.OpenFile(file, os.O_CREATE|os.O_APPEND|os.O_RDWR, os.ModePerm)
// 	if err != nil {
// 		log.Fatalf("create log file failed!")
// 	}
// 	loger = log.New(f, "[Lab 2B]", log.LstdFlags)
// 	// fmt.Println("init over!", file)
// }

func (rf *Raft) rflog(format string, args ...interface{}) {
	if LogOption {
		format = fmt.Sprintf("[%d] ", rf.me) + format
		fmt.Printf(format, args...)
		fmt.Println("")
		// loger.Printf(format, args...)
	}
}

// as each Raft peer becomes aware that successive log entries are
// committed, the peer should send an ApplyMsg to the service (or
// tester) on the same server, via the applyCh passed to Make(). set
// CommandValid to true to indicate that the ApplyMsg contains a newly
// committed log entry.
//
// in part 2D you'll want to send other kinds of messages (e.g.,
// snapshots) on the applyCh, but set CommandValid to false for these
// other uses.
type ApplyMsg struct {
	CommandValid bool
	Command      interface{}
	CommandIndex int

	// For 2D:
	SnapshotValid bool
	Snapshot      []byte
	SnapshotTerm  int
	SnapshotIndex int
}

// A Go object implementing a single Raft peer.
// 实验要求：添加图 2 中描述的信息
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
	log         []LogEntry
	commitIndex int
	lastApplied int
	nextIndex   []int
	matchIndex  []int

	electionStartTime time.Time
	applyChan         chan ApplyMsg
	commitCond        *sync.Cond
}

type RuleState int

const (
	Follower RuleState = iota
	Candidate
	Leader
	Dead
)

// Lab2D 新增 Index 记录日志的索引，因为涉及到了快照，所以真实下标和在日志中的下标不同
// 并且日志的第一项充当记录, 保存 LastIncludedIndex 和 LastIncludedTerm 的信息
type LogEntry struct {
	Command interface{}
	Term    int
	Index   int
}

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

func (s RuleState) String() string {
	switch s {
	case Follower:
		return "Follower"
	case Candidate:
		return "Candidate"
	case Leader:
		return "Leader"
	case Dead:
		return "Dead"
	default:
		panic("unreachable")
	}
}

// return currentTerm and whether this server
// believes it is the leader.
func (rf *Raft) GetState() (int, bool) {

	// Your code here (2A).
	rf.mu.Lock()
	defer rf.mu.Unlock()
	return rf.currentTerm, rf.state == Leader
}

// save Raft's persistent state to stable storage,
// where it can later be retrieved after a crash and restart.
// see paper's Figure 2 for a description of what should be persistent.
// before you've implemented snapshots, you should pass nil as the
// second argument to persister.Save().
// after you've implemented snapshots, pass the current snapshot
// (or nil if there's not yet a snapshot).
func (rf *Raft) persist() {
	// Your code here (2C).
	// Example:
	rf.persister.Save(rf.encodeState(), rf.persister.ReadSnapshot())
}

// 原先的 persist() 构造 raftstate 部分; 因为 Snapshot() 也需要
func (rf *Raft) encodeState() []byte {
	w := new(bytes.Buffer)
	e := labgob.NewEncoder(w)
	e.Encode(rf.currentTerm)
	e.Encode(rf.voteFor)
	e.Encode(rf.log)
	raftstate := w.Bytes()
	return raftstate
}

// restore previously persisted state.
func (rf *Raft) readPersist(data []byte) {
	if data == nil || len(data) < 1 { // bootstrap without any state?
		return
	}
	// Your code here (2C).
	// Example:
	r := bytes.NewBuffer(data)
	d := labgob.NewDecoder(r)
	var currentTerm, voteFor int
	var log []LogEntry
	if d.Decode(&currentTerm) != nil || d.Decode(&voteFor) != nil || d.Decode(&log) != nil {
		return
	} else {
		rf.mu.Lock()
		rf.currentTerm = currentTerm
		rf.voteFor = voteFor
		rf.log = log
		rf.mu.Unlock()
	}
}

// the service says it has created a snapshot that has
// all info up to and including index. this means the
// service no longer needs the log through (and including)
// that index. Raft should now trim its log as much as possible.
func (rf *Raft) Snapshot(index int, snapshot []byte) {
	// Your code here (2D).
	rf.mu.Lock()
	defer rf.mu.Unlock()
	rf.rflog("snapshot index %d", index)
	lastIndex := rf.getFirstIndex()
	if lastIndex >= index {
		// 已经做过快照了
		return
	}
	// 第 0 个日志存快照信息, lastIncludedIndex 和 lastIncludeTerm 就是下标为 index 的日志的信息, 因此裁剪时保留它充当快照信息
	var tmp []LogEntry
	rf.log = append(tmp, rf.log[index-lastIndex:]...)
	rf.log[0].Command = nil
	rf.persister.Save(rf.encodeState(), snapshot)
}

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

func (rf *Raft) InstallSnapshot(args *InstallSnapshotArgs, reply *InstallSnapshotReply) {
	rf.mu.Lock()
	if rf.state == Dead {
		rf.mu.Unlock()
		return
	}
	rf.rflog("receives InstallSnapshot [%v]", args)
	if args.Term > rf.currentTerm {
		rf.rflog("term is out of data in InstallSnapshot")
		rf.becomeFollower(args.Term)
		rf.electionStartTime = time.Now()
	}
	reply.Term = rf.currentTerm
	if args.Term == rf.currentTerm {
		if rf.state != Follower {
			rf.state = Follower
		}
		rf.electionStartTime = time.Now()
		// 是因为延迟得到的过期的快照
		if rf.commitIndex >= args.LastIncludedIndex {
			rf.rflog("receive out of data snapshot, commitIndex: [%d], args.LsdtIncludedIndex: [%d]", rf.commitIndex, args.LastIncludedIndex)
			rf.mu.Unlock()
			return
		}

		// 裁剪日志
		if rf.getLastIndex() <= args.LastIncludedIndex {
			rf.log = make([]LogEntry, 1)
		} else {
			var tmp []LogEntry
			rf.log = append(tmp, rf.log[args.LastIncludedIndex-rf.getFirstIndex():]...)
		}
		rf.log[0].Term = args.LastIncludedTerm
		rf.log[0].Index = args.LastIncludedIndex
		rf.log[0].Command = nil
		rf.persister.Save(rf.encodeState(), args.Snapshot)

		rf.rflog("persist on InstallSnapshot over!, term is %d, log is %d", rf.currentTerm, rf.log)
		rf.lastApplied, rf.commitIndex = args.LastIncludedIndex, args.LastIncludedIndex
		rf.mu.Unlock()
		rf.applyChan <- ApplyMsg{
			SnapshotValid: true,
			Snapshot:      args.Snapshot,
			SnapshotTerm:  args.LastIncludedTerm,
			SnapshotIndex: args.LastIncludedIndex,
		}
		rf.rflog("InstallSnapshot over!")
		return
	}
	rf.mu.Unlock()
}

func (rf *Raft) sendInstallSnapshot(server int, args *InstallSnapshotArgs, reply *InstallSnapshotReply) bool {
	ok := rf.peers[server].Call("Raft.InstallSnapshot", args, reply)
	return ok
}

// example RequestVote RPC arguments structure.
// field names must start with capital letters!
type RequestVoteArgs struct {
	// Your data here (2A, 2B).
	Term         int
	CandidateId  int
	LastLogIndex int
	LastLogTerm  int
}

// example RequestVote RPC reply structure.
// field names must start with capital letters!
type RequestVoteReply struct {
	// Your data here (2A).
	Term        int
	VoteGranted bool
}

// example RequestVote RPC handler.
// Lab2B 完善了请求参数, 所以在投票时需要新增判断
// 当且仅当 任期相等 && (当前节点未投票 || 本来就投给了当前候选者) &&
// (参数中上一日志的任期更大 || 任期相同但是参数中的下标>=自己的下标) 时才会投赞成票
func (rf *Raft) RequestVote(args *RequestVoteArgs, reply *RequestVoteReply) {
	// Your code here (2A, 2B).
	rf.mu.Lock()
	defer rf.mu.Unlock()
	if rf.state == Dead {
		return
	}
	rf.rflog("is requested vote, args [%+v]; currentTerm : %d, voteFor: %d, log: [%v]",
		args, rf.currentTerm, rf.voteFor, rf.log)
	if args.Term > rf.currentTerm {
		rf.rflog("term is out of data in RequestVote")
		// 这个很重要, 2C 检查 bug 发现的, 若当前节点为 Follower 的话不能更新选举定时器开始时间
		// 否则一个不能成为 leader 的节点超时后会不断重置其他节点的选举开始时间然后自己一直增加任期进行选举
		if rf.state != Follower {
			rf.electionStartTime = time.Now()
		}
		rf.becomeFollower(args.Term)
	}

	reply.VoteGranted = false
	lastLogIndex, lastLogTerm, need_persist := rf.getLastIndex(), rf.getLastTerm(), false

	if rf.currentTerm == args.Term &&
		(rf.voteFor == -1 || rf.voteFor == args.CandidateId) &&
		(args.LastLogTerm > lastLogTerm || (args.LastLogTerm == lastLogTerm && args.LastLogIndex >= lastLogIndex)) {

		reply.VoteGranted, rf.electionStartTime = true, time.Now()
		if rf.voteFor == -1 {
			rf.voteFor, need_persist = args.CandidateId, true
		}
	}
	if need_persist {
		rf.persist()
	}
	reply.Term = rf.currentTerm
	rf.rflog("reply in RequestVote [%+v] to [%d]", reply, args.CandidateId)
}

// example code to send a RequestVote RPC to a server.
// server is the index of the target server in rf.peers[].
// expects RPC arguments in args.
// fills in *reply with RPC reply, so caller should
// pass &reply.
// the types of the args and reply passed to Call() must be
// the same as the types of the arguments declared in the
// handler function (including whether they are pointers).
//
// The labrpc package simulates a lossy network, in which servers
// may be unreachable, and in which requests and replies may be lost.
// Call() sends a request and waits for a reply. If a reply arrives
// within a timeout interval, Call() returns true; otherwise
// Call() returns false. Thus Call() may not return for a while.
// A false return can be caused by a dead server, a live server that
// can't be reached, a lost request, or a lost reply.
//
// Call() is guaranteed to return (perhaps after a delay) *except* if the
// handler function on the server side does not return.  Thus there
// is no need to implement your own timeouts around Call().
//
// look at the comments in ../labrpc/labrpc.go for more details.
//
// if you're having trouble getting RPC to work, check that you've
// capitalized all field names in structs passed over RPC, and
// that the caller passes the address of the reply struct with &, not
// the struct itself.
func (rf *Raft) sendRequestVote(server int, args *RequestVoteArgs, reply *RequestVoteReply) bool {
	ok := rf.peers[server].Call("Raft.RequestVote", args, reply)
	return ok
}

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
	// Lab2C 新增的，避免 leader 每次只往前移动一位；若日志很长的话在一段时间内无法达到冲突位置
	ConflictIndex int
	ConflictTerm  int
}

// 首先检查是否 Dead，若 leader 发来的任期更高，当前节点变为 follower（包含了更新任期，定时器，重设投票等操作）
// 若任期相等的话，不管当前节点处于什么状态，重新变为 follower，回复成功
// 回复自己的任期，若自己的更高，当前假 leader 会在处理回复时变为 follower
// Lab2B 新增日志, 接收到完整参数后需要检查自己的日志
// 当且仅当参数中的上一个日志记录小于自己的日志长度并且任期相同时, 回复 true; 否则回复 false
// 当正常时检查参数中的日志跟自己的日志, 找到不同的地方, 用参数中的日志替换自己后续所有日志
// 若发现 leader 的 commitIndex 比自己的大, 更新 commitIndex 并调用 Signal() 通知另一个协程给 applyChan 发送消息
// Lab2C 新增持久化，只要任期，投票，日志修改了一个就进行持久化处理
func (rf *Raft) AppendEntries(args *AppendEntriesArgs, reply *AppendEntriesReply) {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	if rf.state == Dead {
		return
	}
	rf.rflog("receives AppendEntries [Term:%d LeaderId:%d PrevLogIndex:%d PrevLogTerm:%d Entries:%d LeaderCommit:%d]",
		args.Term, args.LeaderId, args.PrevLogIndex, args.PrevLogTerm, len(args.Entries), args.LeaderCommit)

	need_persist := false

	if args.Term > rf.currentTerm {
		rf.rflog("term is out of data in AppendEntries")
		rf.becomeFollower(args.Term)
		rf.electionStartTime = time.Now()
	}

	reply.Success = false
	if args.Term == rf.currentTerm {
		rf.state, rf.electionStartTime = Follower, time.Now()
		if args.PrevLogIndex < rf.getFirstIndex() {
			rf.rflog("receives out of data AppendEntries RPC, args.PrevLogIndex [%d], LastIncludedIndex [%d]", args.PrevLogIndex, rf.getFirstIndex())
			reply.Success, reply.Term = false, 0
			return
		}

		if args.PrevLogIndex < rf.getNextIndex() && args.PrevLogTerm == rf.getTerm(args.PrevLogIndex) {
			reply.Success = true
			insertIndex, argsLogIndex := args.PrevLogIndex+1, 0
			for {
				if insertIndex >= rf.getNextIndex() || argsLogIndex >= len(args.Entries) ||
					rf.getTerm(insertIndex) != args.Entries[argsLogIndex].Term {
					break
				}
				insertIndex++
				argsLogIndex++
			}
			// 并未遍历到参数日志的最后, 将后面的内容拼接上来
			if argsLogIndex < len(args.Entries) {
				rf.log = append(rf.log[:insertIndex-rf.getFirstIndex()], args.Entries[argsLogIndex:]...)
				need_persist = true
				rf.rflog("append logs [%v] in AppendEntries", args.Entries[argsLogIndex:])
			}
			// 检查是否需要提交命令
			if args.LeaderCommit > rf.commitIndex {
				rf.commitIndex = min(rf.getNextIndex()-1, args.LeaderCommit)
				rf.rflog("updates commitIndex into %v", rf.commitIndex)
				rf.commitCond.Signal()
			}
		} else {
			if args.PrevLogIndex >= rf.getNextIndex() {
				reply.ConflictIndex, reply.ConflictTerm = rf.getNextIndex(), -1
			} else {
				reply.ConflictTerm = rf.getTerm(args.PrevLogIndex)
				var ind int
				for ind = args.PrevLogIndex - 1; ind >= rf.getFirstIndex(); ind-- {
					if rf.getTerm(ind) != reply.ConflictTerm {
						break
					}
				}
				reply.ConflictIndex = ind + 1
			}
		}
	}
	if need_persist {
		rf.persist()
	}
	reply.Term = rf.currentTerm
	rf.rflog("reply AppendEntries [%+v] to %d", reply, args.LeaderId)
}

func (rf *Raft) sendHeartBeats(server int, args *AppendEntriesArgs, reply *AppendEntriesReply) bool {
	ok := rf.peers[server].Call("Raft.AppendEntries", args, reply)
	return ok
}

// the service using Raft (e.g. a k/v server) wants to start
// agreement on the next command to be appended to Raft's log. if this
// server isn't the leader, returns false. otherwise start the
// agreement and return immediately. there is no guarantee that this
// command will ever be committed to the Raft log, since the leader
// may fail or lose an election. even if the Raft instance has been killed,
// this function should return gracefully.
//
// the first return value is the index that the command will appear at
// if it's ever committed. the second return value is the current
// term. the third return value is true if this server believes it is
// the leader.
// 添加命令，若非 leader 返回 false，否则启动协议并立即返回（不保证该命令能添加到 Log 中）
// 返回值（命令被提交的索引，当前 term，当前机器是否认为自己是 leader）
func (rf *Raft) Start(command interface{}) (int, int, bool) {

	// Your code here (2B).
	rf.mu.Lock()
	defer rf.mu.Unlock()
	if rf.state != Leader {
		return -1, rf.currentTerm, false
	}
	rf.rflog("receives commond %v", command)
	rf.log = append(rf.log, LogEntry{
		Command: command,
		Term:    rf.currentTerm,
		Index:   rf.getNextIndex(),
	})
	rf.persist()
	return rf.getLastIndex(), rf.currentTerm, true
}

// the tester doesn't halt goroutines created by Raft after each test,
// but it does call the Kill() method. your code can use killed() to
// check whether Kill() has been called. the use of atomic avoids the
// need for a lock.
//
// the issue is that long-running goroutines use memory and may chew
// up CPU time, perhaps causing later tests to fail and generating
// confusing debug output. any goroutine with a long-running loop
// should call killed() to check whether it should stop.
func (rf *Raft) Kill() {
	atomic.StoreInt32(&rf.dead, 1)
	// Your code here, if desired.
	rf.mu.Lock()
	defer rf.mu.Unlock()
	rf.state = Dead
	rf.rflog("becomes dead")
}

func (rf *Raft) killed() bool {
	z := atomic.LoadInt32(&rf.dead)
	return z == 1
}

func (rf *Raft) ticker(state RuleState) {
	if !rf.killed() {
		// Your code here (2A)
		// Check if a leader election should be started.
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

// 超时时间设为 [250, 400]
// 内部利用定时器不断检查，当变为 leader 时直接结束；此外，因为不断在开协程处理，因此可能同时会有多个协程都在运行
// 需要判断运行的协程的任期是否跟当前任期一致，若落后了表明当前协程是上个任期期间运行的定时器，直接结束
// 一直满足条件的话直到超时后开始选举
func (rf *Raft) runElectionTimer() {
	timeout := time.Duration(250+rand.Intn(150)) * time.Millisecond
	rf.mu.Lock()
	nowTerm := rf.currentTerm
	rf.mu.Unlock()
	rf.rflog("election timer start, timeout (%v), now term = (%v)", timeout, nowTerm)

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for !rf.killed() {
		<-ticker.C
		rf.mu.Lock()
		rf.rflog("after %v, timeout is %v, currentTerm [%d], realTerm [%d], state [%s]",
			time.Since(rf.electionStartTime), timeout, nowTerm, rf.currentTerm, rf.state.String())
		if rf.state != Candidate && rf.state != Follower {
			rf.rflog("in runElectionTimer, state change to %s, currentTerm [%d], realTerm [%d]",
				rf.state.String(), nowTerm, rf.currentTerm)
			rf.mu.Unlock()
			return
		}
		if nowTerm != rf.currentTerm {
			rf.rflog("in runElectionTimer, term change from %d to %d, currentTerm [%d], realTerm [%d]",
				nowTerm, rf.currentTerm, nowTerm, rf.currentTerm)
			rf.mu.Unlock()
			return
		}
		// 若超时了则开始选举
		if duration := time.Since(rf.electionStartTime); duration >= timeout {
			rf.rflog("timeed out !! timer after %v, currentTerm [%d], realTerm [%d]",
				time.Since(rf.electionStartTime), nowTerm, rf.currentTerm)
			rf.startElection()
			rf.mu.Unlock()
			continue
		}
		rf.mu.Unlock()
	}
}

// 开始选举需要修改状态，增加任期，给自己投票并重置选举定时器的开始时间
// 开启多个协程给其它 peers 发送 sendRequestVote RPC，等待回复
// 收到回复后判断回复的合法性，必须满足当前节点状态仍为 Candidate 以及当前节点的任期等于发送 RPC 时的任期
// 回复成功就累计投票，若多于半数赞同就可以变成 leader
// 若发现回复信息的任期更大，表明出现 leader 了，变为 follower，重置选举开始时间
// 注意需要运行新的选举定时器以避免此次选举失败
// Lab2B 新增日志, 发送完整 RequestVoteArgs
func (rf *Raft) startElection() {
	// 固定住状态, 避免收到回复后状态改变然后发送了错误的 RequestVote RPC
	rf.currentTerm += 1
	rf.state, rf.voteFor, rf.electionStartTime = Candidate, rf.me, time.Now()
	rf.persist()
	rf.rflog("becomes Candidate, start election! now term is %d", rf.currentTerm)
	receivedVotes := 1

	args := RequestVoteArgs{
		Term:         rf.currentTerm,
		CandidateId:  rf.me,
		LastLogIndex: rf.getLastIndex(),
		LastLogTerm:  rf.getLastTerm(),
	}

	for peerId := range rf.peers {
		if peerId == args.CandidateId {
			continue
		}
		go func(peerId int) {
			var reply RequestVoteReply
			if succ := rf.sendRequestVote(peerId, &args, &reply); succ {
				rf.rflog("receive requestVote reply [%+v]", reply)
				rf.mu.Lock()
				defer rf.mu.Unlock()

				// 应该先判断回复是否合法，必须满足 1. 当前节点仍是 Candidate 2. 当前任期仍等于发送RPC时的任期
				if rf.state == Candidate && rf.currentTerm == args.Term {
					if reply.VoteGranted {
						receivedVotes += 1
						if receivedVotes*2 >= len(rf.peers)+1 {
							rf.rflog("wins the selection, becomes leader!")
							rf.becomeLeader()
							rf.runHeartBeats()
						}
					} else if reply.Term > rf.currentTerm {
						rf.rflog("receive bigger term in reply, maybe out of data")
						rf.becomeFollower(reply.Term)
						rf.electionStartTime = time.Now()
					}
				}
			}
		}(peerId)
	}
	// 2C 发现的 bug, 因为锁的抢占问题可能 ticker() 中获取状态时已经变成 leader 了，进而存在两个心跳计时器，因此指定开启哪个定时器
	go rf.ticker(Follower)
}

// 修改状态，任期，清除投票结果，运行新的选举定时器
// Lab2C 进行持久化
func (rf *Raft) becomeFollower(term int) {
	rf.state = Follower
	rf.currentTerm = term
	rf.voteFor = -1
	rf.rflog("becomes follower at term [%d]", term)
	rf.persist()
	go rf.ticker(Follower)
}

// 修改状态，重设 nextIndex[i], 启动新的 ticker()，原来的 ticker() 会因为自身状态的改变而主动退出
func (rf *Raft) becomeLeader() {
	rf.state = Leader
	rf.rflog("becomes leader, term = [%d]", rf.currentTerm)
	nextIndex := rf.getNextIndex()
	for i := range rf.peers {
		rf.nextIndex[i] = nextIndex
		rf.matchIndex[i] = 0
	}
	go rf.ticker(Leader)
}

// 心跳定时器，100ms，当当前节点的状态发生改变，不再是 leader 后结束
// 简化了锁的控制流程，发送心跳直接放到锁里了
func (rf *Raft) heartBeatsTimer() {
	rf.mu.Lock()
	nowTerm := rf.currentTerm
	rf.mu.Unlock()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for !rf.killed() {
		<-ticker.C
		rf.mu.Lock()
		if rf.state != Leader || rf.currentTerm != nowTerm {
			rf.mu.Unlock()
			return
		}
		rf.runHeartBeats()
		rf.mu.Unlock()
	}
}

// Lab2B 中需要完善发送的 AppendEntriesArgs, 发送完整信息
// Lab2D 引入快照后需要检查发送的第一个日志是否被裁减，若被裁减则发送快照；否则走 Lab2B 的流程发送日志
// 注意只要收到回复后都应该先判断回复的合法性（1. 当前节点仍是 Leader 2. 当前任期仍等于发送 RPC 时的任期）
func (rf *Raft) runHeartBeats() {
	if rf.state != Leader {
		rf.rflog("is not a leader any more!")
		return
	}
	currentTerm := rf.currentTerm
	rf.rflog("ticker!!!--------run runHeartBeats()")
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
					rf.rflog("send snapshot to %d, nextIndex is [%d] but lastIncludedIndex is [%d]", peerId, rf.nextIndex[peerId], firstIndex)
					args := InstallSnapshotArgs{
						Term:              currentTerm,
						LeaderId:          rf.me,
						LastIncludedIndex: firstIndex,
						LastIncludedTerm:  rf.getFirstTerm(),
						Snapshot:          rf.persister.ReadSnapshot(),
					}
					rf.mu.Unlock()
					var reply InstallSnapshotReply
					rf.rflog("sending InstallSnapshotArgs to [%v], args = [%+v]", peerId, args)
					if rf.sendInstallSnapshot(peerId, &args, &reply) {
						rf.rflog("receive InstallSnapshot reply [%+v]", reply)
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
					rf.rflog("sending AppendEntries to [%v], args = [%+v]", peerId, args)
					if rf.sendHeartBeats(peerId, &args, &reply) {
						rf.rflog("receive AppendEntries reply [%+v]", reply)
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

// 处理 InstallSnapshotReply，先判断回复的合法性
// 若 reply.Term 更大，变为 follower，重置选举定时器开始时间；否则更新 matchIndex (注意取大) 和 nextIndex
func (rf *Raft) handleInstallSnapshotRPCResponse(peerId int, args *InstallSnapshotArgs, reply *InstallSnapshotReply) {
	// 判断回复的合法性，必须满足 1. 当前节点仍是 Leader 2. 当前任期仍等于发送 RPC 时的任期
	if rf.state == Leader && rf.currentTerm == args.Term {
		if reply.Term == rf.currentTerm {
			rf.rflog("receives reply from [%v], nextIndex changes from [%v] to [%v]",
				peerId, rf.nextIndex[peerId], args.LastIncludedIndex+1)
			rf.matchIndex[peerId] = max(rf.matchIndex[peerId], args.LastIncludedIndex)
			rf.nextIndex[peerId] = rf.matchIndex[peerId] + 1
		} else if reply.Term > rf.currentTerm {
			rf.rflog("receive bigger term in reply, transforms to follower")
			rf.becomeFollower(reply.Term)
			rf.electionStartTime = time.Now()
		}
	}
}

// 处理 AppendEntriesReply，先判断回复的合法性；并决定是否需要继续发送心跳 (当且仅当收到的reply.Success == false)
// 若 reply.Term 更大，变为 follower，重置选举定时器开始时间；
// 否则若 reply.Success 为 true, 更新 matchIndex (注意取大) 和 nextIndex;随后检查是否需要进一步提交新命令
// 若 reply.Success 为 false，返回 true，表示再次发送心跳；返回前根据冲突信息更新 nextIndex
func (rf *Raft) handleAppendEntriesRPCResponse(peerId int, args *AppendEntriesArgs, reply *AppendEntriesReply) bool {
	// 判断回复的合法性，必须满足 1. 当前节点仍是 Leader 2. 当前任期仍等于发送 RPC 时的任期
	if rf.state == Leader && rf.currentTerm == args.Term {
		if reply.Term == rf.currentTerm {
			if reply.Success {
				rf.matchIndex[peerId] = max(rf.matchIndex[peerId], args.PrevLogIndex+len(args.Entries))
				rf.nextIndex[peerId] = rf.matchIndex[peerId] + 1
				rf.rflog("receives reply from [%v], nextIndex := [%v], matchIndex := [%v]",
					peerId, rf.nextIndex[peerId], rf.matchIndex[peerId])
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
					rf.rflog("updates commitIndex from %v to %v", savedCommitIndex, rf.commitIndex)
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
				rf.rflog("receives reply from [%v] failed, nextIndex changes to [%d]", peerId, rf.nextIndex[peerId])
				return true
			}
		} else if reply.Term > rf.currentTerm {
			rf.rflog("receive bigger term in reply, transforms to follower")
			rf.becomeFollower(reply.Term)
			rf.electionStartTime = time.Now()
		}
	}
	return false
}

// 提交命令，当 rf.lastApplied >= rf.commitIndex 时调用 Wait(), 等待其他协程发送 Signal() 信号通知
func (rf *Raft) commitCommand() {
	for !rf.killed() {
		rf.mu.Lock()
		for rf.lastApplied >= rf.commitIndex {
			rf.commitCond.Wait()
		}

		logEntries := make([]LogEntry, rf.commitIndex-rf.lastApplied)
		firstIndex := rf.getFirstIndex()
		commitindex := rf.commitIndex
		copy(logEntries, rf.log[rf.lastApplied+1-firstIndex:rf.commitIndex+1-firstIndex])
		rf.rflog("commits log from %d (%d) to %d (%d)", rf.lastApplied-firstIndex, rf.lastApplied, rf.commitIndex-firstIndex, rf.commitIndex)
		rf.lastApplied = max(rf.lastApplied, commitindex)

		rf.mu.Unlock()
		rf.rflog("commit log: [%v]", logEntries)
		for _, entry := range logEntries {
			rf.applyChan <- ApplyMsg{
				CommandValid: true,
				Command:      entry.Command,
				CommandIndex: entry.Index,
			}
		}
		rf.rflog("commits log from over !!")
	}
}

// the service or tester wants to create a Raft server. the ports
// of all the Raft servers (including this one) are in peers[]. this
// server's port is peers[me]. all the servers' peers[] arrays
// have the same order. persister is a place for this server to
// save its persistent state, and also initially holds the most
// recent saved state, if any. applyCh is a channel on which the
// tester or service expects Raft to send ApplyMsg messages.
// Make() must return quickly, so it should start goroutines
// for any long-running work.
func Make(peers []*labrpc.ClientEnd, me int,
	persister *Persister, applyCh chan ApplyMsg) *Raft {
	rf := &Raft{}
	rf.peers = peers
	rf.persister = persister
	rf.me = me

	// Your initialization code here (2A, 2B, 2C).
	rf.currentTerm = 0
	rf.voteFor = -1
	rf.state = Follower
	rf.dead = 0
	rf.electionStartTime = time.Now()

	// 设置第一个为空条目, 表示 lastIncludedIndex 和 lastIncludedTerm, 初始化都为 0
	rf.log = make([]LogEntry, 1)

	// initialize from state persisted before a crash
	rf.readPersist(persister.ReadRaftState())

	// 读取的第一个日志记录了之前的 lastIncludedIndex 信息, 若节点崩溃应将 commitIndex 和 lastApplied 设为它
	// 同理 rf.nextIndex[] 和 rf.matchIndex[] 也应做相应的修改
	rf.commitIndex = rf.getFirstIndex()
	rf.lastApplied = rf.getFirstIndex()
	rf.nextIndex = make([]int, len(peers))
	rf.matchIndex = make([]int, len(peers))
	for i := range rf.nextIndex {
		rf.nextIndex[i] = rf.getNextIndex()
		rf.matchIndex[i] = 0
	}
	rf.applyChan = applyCh
	rf.commitCond = sync.NewCond(&rf.mu)

	// start ticker goroutine to start elections
	go rf.ticker(Follower)

	// 开启协程检查是否需要提交命令
	go rf.commitCommand()

	return rf
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
