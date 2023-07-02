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
	//	"bytes"

	"math/rand"

	// "rand"
	"sync"
	"sync/atomic"
	"time"

	//	"6.5840/labgob"
	"fmt"

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
	// leaderId           int
	applyChan           chan ApplyMsg
	notifyNewCommitChan chan struct{}
}

type RuleState int

const (
	Follower RuleState = iota
	Candidate
	Leader
	Dead
)

type LogEntry struct {
	Command interface{}
	Term    int
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
	// w := new(bytes.Buffer)
	// e := labgob.NewEncoder(w)
	// e.Encode(rf.xxx)
	// e.Encode(rf.yyy)
	// raftstate := w.Bytes()
	// rf.persister.Save(raftstate, nil)
}

// restore previously persisted state.
func (rf *Raft) readPersist(data []byte) {
	if data == nil || len(data) < 1 { // bootstrap without any state?
		return
	}
	// Your code here (2C).
	// Example:
	// r := bytes.NewBuffer(data)
	// d := labgob.NewDecoder(r)
	// var xxx
	// var yyy
	// if d.Decode(&xxx) != nil ||
	//    d.Decode(&yyy) != nil {
	//   error...
	// } else {
	//   rf.xxx = xxx
	//   rf.yyy = yyy
	// }
}

// the service says it has created a snapshot that has
// all info up to and including index. this means the
// service no longer needs the log through (and including)
// that index. Raft should now trim its log as much as possible.
func (rf *Raft) Snapshot(index int, snapshot []byte) {
	// Your code here (2D).

}

// example RequestVote RPC arguments structure.
// field names must start with capital letters!
// 看 Fig.2
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
	lastLogIndex := len(rf.log) - 1
	lastLogTerm := rf.log[lastLogIndex].Term
	if args.Term > rf.currentTerm {
		rf.rflog("term is out of data in RequestVote")
		// rf.mu.Unlock()
		// 直接设置任期以及投票可以 go rf.becomeFollower(args.Term)，下面的回复也可以正常运行
		// 若顺序执行的话，重置都在函数内部，但需要先解锁, becomeFollower, 再加锁,容易出错
		// rf.currentTerm = args.Term
		// rf.voteFor = -1
		// rf.state = Follower
		// rf.electionStartTime = time.Now()
		// go rf.ticker()
		rf.becomeFollower(args.Term)
		// 若用 go 的话当前已经投过票了，函数结束后又会变为未投票的状态
		// go rf.becomeFollower(args.Term)
	}
	// rf.rflog("in RequestVote, now log is [%v]", rf.log)
	reply.VoteGranted = false
	if rf.currentTerm == args.Term &&
		(rf.voteFor == -1 || rf.voteFor == args.CandidateId) &&
		(args.LastLogTerm > lastLogTerm || (args.LastLogTerm == lastLogTerm && args.LastLogIndex >= lastLogIndex)) {
		reply.VoteGranted = true
		rf.voteFor = args.CandidateId
		rf.electionStartTime = time.Now()
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
}

// 首先检查是否 Dead，若 leader 发来的任期更高，当前节点变为 follower（包含了更新任期，定时器，重设投票等操作）
// 若任期相等的话，不管当前节点处于什么状态，重新变为 follower，回复成功
// 回复自己的任期，若自己的更高，当前假 leader 会在处理回复时变为 follower

// Lab2B 新增日志, 接收到完整参数后需要检查自己的日志
// 当且仅当参数中的上一个日志记录小于自己的日志长度并且任期相同时, 回复 true; 否则回复 false
// 当正常时检查参数中的日志跟自己的日志, 找到不同的地方, 用参数中的日志替换自己后续所有日志
// 若发现 leader 的 commitIndex 比自己的大, 更新 commitIndex 并给 notifyNewCommitChan 发送消息
func (rf *Raft) AppendEntries(args *AppendEntriesArgs, reply *AppendEntriesReply) {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	if rf.state == Dead {
		return
	}
	rf.rflog("receives AppendEntries [%+v]", args)

	if args.Term > rf.currentTerm {
		rf.rflog("term is out of data in AppendEntries")
		// rf.currentTerm = args.Term
		// rf.electionStartTime = time.Now()
		// rf.state = Follower
		rf.becomeFollower(args.Term)
	}

	reply.Success = false
	if args.Term == rf.currentTerm {
		if rf.state != Follower {
			rf.state = Follower
			// rf.rflog("transform from %s to Follower", rf.state)
			// 同一个任期内若投过票的话这样会变为-1
			// go rf.becomeFollower(args.Term)
		}
		rf.electionStartTime = time.Now()
		// rf.rflog("election timer reset to %v!", time.Since(rf.electionStartTime))
		if args.PrevLogIndex < len(rf.log) && args.PrevLogTerm == rf.log[args.PrevLogIndex].Term {
			reply.Success = true
			insertIndex := args.PrevLogIndex + 1
			argsLogIndex := 0
			for {
				if insertIndex >= len(rf.log) || argsLogIndex >= len(args.Entries) ||
					rf.log[insertIndex].Term != args.Entries[argsLogIndex].Term {
					break
				}
				insertIndex++
				argsLogIndex++
			}
			// 并未遍历到参数日志的最后, 将后面的内容拼接上来
			if argsLogIndex < len(args.Entries) {
				rf.log = append(rf.log[:insertIndex], args.Entries[argsLogIndex:]...)
				rf.rflog("append logs [%v] in AppendEntries", args.Entries[argsLogIndex:])
			}
			// 检查是否需要提交命令
			if args.LeaderCommit > rf.commitIndex {
				rf.commitIndex = min(len(rf.log)-1, args.LeaderCommit)
				rf.rflog("updates commitIndex into %v", rf.commitIndex)
				rf.notifyNewCommitChan <- struct{}{}
			}
		}
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
		Term:    rf.currentTerm,
		Command: command,
	})
	// rf.rflog("return result: [%v, %v]", len(rf.log)-1, rf.currentTerm)
	return len(rf.log) - 1, rf.currentTerm, true
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
	// rf.mu.Lock()
	// defer rf.mu.Unlock()
	z := atomic.LoadInt32(&rf.dead)
	return z == 1
}

func (rf *Raft) ticker(state RuleState) {
	if !rf.killed() {
		// Your code here (2A)
		// Check if a leader election should be started.
		// rf.mu.Lock()
		// state := rf.state
		// rf.mu.Unlock()
		switch state {
		case Follower:
			rf.runElectionTimer()
		case Candidate:
			rf.runElectionTimer()
		case Leader:
			rf.heartBeatsTimer()
		}
		// pause for a random amount of time between 50 and 350
		// milliseconds.
		// ms := 50 + (rand.Int63() % 300)
		// time.Sleep(time.Duration(ms) * time.Millisecond)
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
		// rf.rflog("after %v, timeout is %v, currentTerm [%d], realTerm [%d], state [%s]",
		// 	time.Since(rf.electionStartTime), timeout, nowTerm, rf.currentTerm, rf.state.String())
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
			rf.mu.Unlock()
			rf.startElection()
			continue
		}
		rf.mu.Unlock()
	}
}

// 开始选举需要修改状态，增加任期，给自己投票并重置选举定时器的开始时间
// 开启多个协程给其它 peers 发送 sendRequestVote RPC，等待回复
//
//	若统计结果时发现自己已经不在是 Candidate，表明超时了但仍未收到多数投票，直接返回
//	若发现回复信息的任期更大，表明出现 leader 了，变为 follower
//	一切正常就累计投票，若多于半数赞同就可以变成 leader
//
// 注意需要运行新的选举定时器以避免此次选举失败

// Lab2B 新增日志, 发送完整 RequestVoteArgs
func (rf *Raft) startElection() {
	rf.mu.Lock()
	rf.state = Candidate
	rf.currentTerm += 1
	rf.voteFor = rf.me
	rf.electionStartTime = time.Now()
	currentTerm := rf.currentTerm
	lastLogIndex := len(rf.log) - 1
	lastLogTerm := rf.log[lastLogIndex].Term
	candidateId := rf.me
	rf.mu.Unlock()
	rf.rflog("becomes Candidate, start election! now term is %d", currentTerm)
	receivedVotes := 1

	for peerId := range rf.peers {
		if peerId == rf.me {
			continue
		}
		go func(peerId int) {

			args := RequestVoteArgs{
				Term:         currentTerm,
				CandidateId:  candidateId,
				LastLogIndex: lastLogIndex,
				LastLogTerm:  lastLogTerm,
			}
			var reply RequestVoteReply
			if succ := rf.sendRequestVote(peerId, &args, &reply); succ {
				rf.rflog("receive requestVote reply [%+v]", reply)

				rf.mu.Lock()
				defer rf.mu.Unlock()
				if rf.state != Candidate {
					rf.rflog("transforms to state [%s] when waiting for reply", rf.state)
					return
				} else if reply.Term > currentTerm {
					rf.rflog(" receive bigger term in reply, transforms to follower")
					// rf.state = Follower
					rf.becomeFollower(reply.Term)
					return
				} else if reply.Term == currentTerm && reply.VoteGranted { // 正确的响应，查看是否同意
					receivedVotes += 1
					if receivedVotes*2 >= len(rf.peers)+1 {
						rf.rflog("wins the selection, becomes leader!")
						// rf.state = Leader
						rf.becomeLeader()
					}
					return
				}
			}
		}(peerId)
	}
	// 有一个问题在于上面和 ticker() 都上锁了，上面选举成功后，状态变为了 leader 并启动了 heartBeatTimer,
	// 然后这个 ticker() 获得锁的时候状态已经是 leader 了，导致同时有两个 heartBeatTimer，之后每个数据都会发送两次
	go rf.ticker(Follower)
}

// 修改状态，任期，更新选举开始时间，清除投票结果，运行新的选举定时器
func (rf *Raft) becomeFollower(term int) {
	// rf.mu.Lock()
	rf.state = Follower
	rf.currentTerm = term
	// rf.electionStartTime = time.Now()
	rf.voteFor = -1
	// rf.mu.Unlock()
	rf.rflog("becomes follower at term [%d]", term)
	go rf.ticker(Follower)
}

// 修改状态，启动 ticker()，原来的 ticker() 会因为自身状态的改变而主动退出
func (rf *Raft) becomeLeader() {
	// rf.mu.Lock()
	// defer rf.mu.Unlock()
	rf.state = Leader
	rf.rflog("becomes leader, term = [%d]", rf.currentTerm)
	for i := range rf.peers {
		rf.nextIndex[i] = len(rf.log)
		rf.matchIndex[i] = 0
	}
	go rf.ticker(Leader)
}

// 心跳定时器，100ms
// 当当前节点的状态发生改变，不再是 leader 后结束
func (rf *Raft) heartBeatsTimer() {
	rf.mu.Lock()
	nowTerm := rf.currentTerm
	rf.mu.Unlock()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for !rf.killed() {
		rf.runHeartBeats()
		<-ticker.C
		rf.mu.Lock()
		// rf.rflog("in heartBeatsTimer, ticker!, time is [%v]", time.Since(rf.electionStartTime))
		if rf.state != Leader || rf.currentTerm != nowTerm {
			rf.mu.Unlock()
			return
		}
		rf.mu.Unlock()
	}
}

// 首先检查当前状态是否仍是 leader，因为可能这是上一个任期的 ticker() 调用的函数，也可能因为网络分区的缘故，其它地方有任期更高的 leader
// 给其它节点发送 AppendEntriesArgs，等待回复
// 若回复的任期更高，当前节点重新变回 follower

// Lab2B 中需要完善发送的 AppendEntriesArgs, 发送完整信息
// 收到请求后, 应该统计结果来决定是否增加 commitIndex
func (rf *Raft) runHeartBeats() {
	rf.mu.Lock()
	if rf.state != Leader {
		rf.rflog("is not a leader any more!")
		rf.mu.Unlock()
		return
	}
	id := rf.me
	currentTerm := rf.currentTerm
	rf.mu.Unlock()

	rf.rflog("ticker!!!--------run runHeartBeats()")
	for peerId := range rf.peers {
		if peerId == id {
			continue
		}
		go func(peerId int) {
			for !rf.killed() {
				rf.mu.Lock()
				// prevLogIndex, nowLogIndex := rf.nextIndex[peerId]-1, rf.nextIndex[peerId]
				// prevLogIndex, nowLogIndex := rf.nextIndex[peerId]-1, rf.nextIndex[peerId]
				prevLogIndex, nowLogIndex := min(rf.nextIndex[peerId]-1, len(rf.log)-1), min(rf.nextIndex[peerId], len(rf.log))
				// if len(rf.log) > 1 {
				// }
				if rf.state != Leader {
					rf.mu.Unlock()
					return
				}
				entries := make([]LogEntry, len(rf.log[nowLogIndex:]))
				copy(entries, rf.log[nowLogIndex:])
				args := AppendEntriesArgs{
					Term:         currentTerm,
					LeaderId:     rf.me,
					PrevLogIndex: prevLogIndex,
					PrevLogTerm:  rf.log[prevLogIndex].Term,
					Entries:      entries,
					LeaderCommit: rf.commitIndex,
				}
				rf.mu.Unlock()
				var reply AppendEntriesReply
				rf.rflog("sending AppendEntries to [%v], args = [%+v]", peerId, args)
				if succ := rf.sendHeartBeats(peerId, &args, &reply); succ {
					rf.rflog("receive AppendEntries reply [%+v]", reply)
					rf.mu.Lock()
					if reply.Term > currentTerm {
						rf.rflog("receive bigger term in reply, transforms to follower")
						rf.becomeFollower(reply.Term)
						rf.electionStartTime = time.Now()
						rf.mu.Unlock()
						return
					}
					// defer rf.mu.Unlock()
					// 收到理想的回复, 统计投票结果
					if rf.state == Leader && currentTerm == reply.Term && !rf.killed() {
						if reply.Success {
							rf.matchIndex[peerId] = prevLogIndex + len(args.Entries)
							rf.nextIndex[peerId] = rf.matchIndex[peerId] + 1
							rf.rflog("receives reply from [%v], nextIndex := [%v], matchIndex := [%v]",
								peerId, rf.nextIndex[peerId], rf.matchIndex[peerId])
							// 统计投票结果, 更新 commitIndex
							savedCommitIndex := rf.commitIndex
							for i := rf.commitIndex + 1; i < len(rf.log); i++ {
								if rf.log[i].Term == rf.currentTerm {
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
							// rf.rflog("savedCommitIndex is [%d], commitIndex change to [%d]", savedCommitIndex, rf.commitIndex)
							if rf.commitIndex != savedCommitIndex {
								rf.rflog("updates commitIndex from %v to %v", savedCommitIndex, rf.commitIndex)
								rf.notifyNewCommitChan <- struct{}{}
								// rf.rflog("updates over!")
							}
						} else if rf.nextIndex[peerId] > 1 {
							rf.nextIndex[peerId] -= 1
							rf.rflog("receives reply from [%v] failed", peerId)
							rf.mu.Unlock()
							continue
						}
					}
					rf.mu.Unlock()
				}
				return
			}
		}(peerId)
	}
}

// 监听 notifyNewCommitChan, 当有东西发到通道后, 向 rf.applyChan 提交命令
func (rf *Raft) commitCommand() {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-rf.notifyNewCommitChan:
			ticker.Reset(10 * time.Millisecond)
			// rf.rflog("receive info")
			rf.mu.Lock()
			savedLastApplied := rf.lastApplied
			var logEntries []LogEntry
			if rf.commitIndex > rf.lastApplied {
				logEntries = rf.log[rf.lastApplied+1 : rf.commitIndex+1]
				rf.lastApplied = rf.commitIndex
			}
			rf.rflog("commits log from %d to %d", savedLastApplied, rf.lastApplied)
			rf.mu.Unlock()
			for i, entry := range logEntries {
				rf.applyChan <- ApplyMsg{
					CommandValid: true,
					Command:      entry.Command,
					CommandIndex: savedLastApplied + i + 1,
				}
			}
			ticker.Reset(10 * time.Millisecond)
		case <-ticker.C:
			if rf.killed() {
				return
			}
		}
	}
	// for range rf.notifyNewCommitChan {
	// 	rf.rflog("receive info")
	// 	rf.mu.Lock()
	// 	savedLastApplied := rf.lastApplied
	// 	var logEntries []LogEntry
	// 	if rf.commitIndex > rf.lastApplied {
	// 		logEntries = rf.log[rf.lastApplied+1 : rf.commitIndex+1]
	// 		rf.lastApplied = rf.commitIndex
	// 	}
	// 	rf.rflog("commits log [%v] from %d to %d", logEntries, savedLastApplied, rf.lastApplied)
	// 	rf.mu.Unlock()
	// 	for i, entry := range logEntries {
	// 		rf.applyChan <- ApplyMsg{
	// 			CommandValid: true,
	// 			Command:      entry.Command,
	// 			CommandIndex: savedLastApplied + i + 1,
	// 		}
	// 	}
	// 	// rf.rflog("commits over!")
	// }
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
	rf.log = []LogEntry{}
	rf.log = append(rf.log, LogEntry{
		Term:    0,
		Command: "MIT6.5840 !",
	})
	rf.commitIndex = 0
	rf.lastApplied = 0
	rf.nextIndex = make([]int, len(peers))
	rf.matchIndex = make([]int, len(peers))
	for i := range rf.nextIndex {
		rf.nextIndex[i] = 1
		rf.matchIndex[i] = 0
	}
	rf.applyChan = applyCh
	rf.notifyNewCommitChan = make(chan struct{})

	// initialize from state persisted before a crash
	rf.readPersist(persister.ReadRaftState())

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
