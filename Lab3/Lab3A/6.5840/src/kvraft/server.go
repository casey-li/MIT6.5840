package kvraft

import (
	"log"
	"sync"
	"sync/atomic"
	"time"

	"6.5840/labgob"
	"6.5840/labrpc"
	"6.5840/raft"
)

const Debug = false

func DPrintf(format string, a ...interface{}) (n int, err error) {
	if Debug {
		log.Printf(format, a...)
	}
	return
}

const ExecuteTimeout = 500 * time.Millisecond

type Op struct {
	// Your definitions here.
	// Field names must start with capital letters,
	// otherwise RPC will break.
	ClientId  int64  // 客户端 Id
	RequestId int64  // 请求 Id
	OpType    string // 操作类别
	Key       string
	Value     string
}

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
	lastRequestMap map[int64]int64   // 保存每个客户端对应的最近一次请求的Id
}

// Get 和 PutAppend 都是先封装 OP，再调用 Raft 的 Start()
func (kv *KVServer) Get(args *GetArgs, reply *GetReply) {
	// Your code here.
	op := Op{
		ClientId:  args.ClientId,
		RequestId: args.RequestId,
		OpType:    "Get",
		Key:       args.Key,
	}
	index, term, isLeader := kv.rf.Start(op)
	if !isLeader {
		reply.Err = ErrWrongLeader
		return
	}
	kv.mu.Lock()
	id := kv.me
	DPrintf("[%d] send Get to leader, log index [%d], log term [%d], args [%v]", id, index, term, args)
	waitChan, exist := kv.waitChMap[index]
	if !exist {
		kv.waitChMap[index] = make(chan *Op, 1)
		waitChan = kv.waitChMap[index]
	}
	DPrintf("[%d] wait for timeout", id)
	kv.mu.Unlock()
	select {
	case res := <-waitChan:
		DPrintf("[%d] receive res from waitChan [%v]", id, res)
		reply.Value, reply.Err = res.Value, OK
		currentTerm, stillLeader := kv.rf.GetState()
		if !stillLeader || currentTerm != term {
			DPrintf("[%d] has accident, stillLeader [%t], term [%d], currentTerm [%d]", id, stillLeader, term, currentTerm)
			reply.Err = ErrWrongLeader
		}
	case <-time.After(ExecuteTimeout):
		DPrintf("[%d] timeout!", id)
		reply.Err = ErrWrongLeader
	}

	kv.mu.Lock()
	delete(kv.waitChMap, index)
	kv.mu.Unlock()
}

func (kv *KVServer) PutAppend(args *PutAppendArgs, reply *PutAppendReply) {
	// Your code here.
	kv.mu.Lock()
	id := kv.me
	if kv.isInvalidRequest(args.ClientId, args.RequestId) {
		DPrintf("[%d] receive out of data request", id)
		reply.Err = OK
		kv.mu.Unlock()
		return
	}
	kv.mu.Unlock()

	op := Op{
		ClientId:  args.ClientId,
		RequestId: args.RequestId,
		OpType:    args.Op,
		Key:       args.Key,
		Value:     args.Value,
	}
	index, term, isLeader := kv.rf.Start(op)
	if !isLeader {
		reply.Err = ErrWrongLeader
		return
	}
	kv.mu.Lock()
	DPrintf("[%d] send PutAppend to leader, log index [%d], log term [%d], args [%v]", id, index, term, args)
	waitChan, exist := kv.waitChMap[index]
	if !exist {
		kv.waitChMap[index] = make(chan *Op, 1)
		waitChan = kv.waitChMap[index]
	}
	DPrintf("[%d] wait for timeout", id)
	kv.mu.Unlock()

	select {
	case res := <-waitChan:
		DPrintf("[%d] receive res from waitChan [%v]", id, res)
		reply.Err = OK
		currentTerm, stillLeader := kv.rf.GetState()
		if !stillLeader || currentTerm != term {
			DPrintf("[%d] has accident, stillLeader [%t], term [%d], currentTerm [%d]", id, stillLeader, term, currentTerm)
			reply.Err = ErrWrongLeader
		}
	case <-time.After(ExecuteTimeout):
		DPrintf("[%d] timeout!", id)
		reply.Err = ErrWrongLeader
	}

	kv.mu.Lock()
	delete(kv.waitChMap, index)
	kv.mu.Unlock()

}

// the tester calls Kill() when a KVServer instance won't
// be needed again. for your convenience, we supply
// code to set rf.dead (without needing a lock),
// and a killed() method to test rf.dead in
// long-running loops. you can also add your own
// code to Kill(). you're not required to do anything
// about this, but it may be convenient (for example)
// to suppress debug output from a Kill()ed instance.
func (kv *KVServer) Kill() {
	atomic.StoreInt32(&kv.dead, 1)
	kv.rf.Kill()
	// Your code here, if desired.
}

func (kv *KVServer) killed() bool {
	z := atomic.LoadInt32(&kv.dead)
	return z == 1
}

// 监听 Raft 提交的 applyMsg, 根据 applyMsg 的类别执行不同的操作
// 为命令的话，必执行，执行完后检查是否需要给 waitCh 发通知
func (kv *KVServer) applier() {
	for !kv.killed() {
		applyMsg := <-kv.applyCh
		kv.mu.Lock()
		DPrintf("[%d] receives applyMsg [%v]", kv.me, applyMsg)
		// 根据收到的是命令还是快照来决定相应的操作，3A仅需处理命令
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

// 执行命令，若为重复命令并且不是 Get 的话，直接返回即可
// 否则根据 OpType 执行命令并更新该客户端最近一次请求的Id
func (kv *KVServer) execute(op *Op) {
	DPrintf("[%d] apply command [%v] success", kv.me, op)
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

// 给 waitCh 发送通知，让其生成响应。必须在发送前检查一下 waitCh 是否关闭
func (kv *KVServer) notifyWaitCh(index int, op *Op) {
	DPrintf("[%d] notifyWaitCh [%d]", kv.me, index)
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

// servers[] contains the ports of the set of
// servers that will cooperate via Raft to
// form the fault-tolerant key/value service.
// me is the index of the current server in servers[].
// the k/v server should store snapshots through the underlying Raft
// implementation, which should call persister.SaveStateAndSnapshot() to
// atomically save the Raft state along with the snapshot.
// the k/v server should snapshot when Raft's saved state exceeds maxraftstate bytes,
// in order to allow Raft to garbage-collect its log. if maxraftstate is -1,
// you don't need to snapshot.
// StartKVServer() must return quickly, so it should start goroutines
// for any long-running work.
func StartKVServer(servers []*labrpc.ClientEnd, me int, persister *raft.Persister, maxraftstate int) *KVServer {
	// call labgob.Register on structures you want
	// Go's RPC library to marshall/unmarshall.
	labgob.Register(Op{})

	kv := new(KVServer)
	kv.me = me
	kv.maxraftstate = maxraftstate

	// You may need initialization code here.

	kv.applyCh = make(chan raft.ApplyMsg)
	kv.rf = raft.Make(servers, me, persister, kv.applyCh)

	// You may need initialization code here.
	kv.KVDB = make(map[string]string)
	kv.waitChMap = make(map[int]chan *Op)
	kv.lastRequestMap = make(map[int64]int64)

	go kv.applier()
	return kv
}
