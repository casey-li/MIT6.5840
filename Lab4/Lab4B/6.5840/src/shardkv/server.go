package shardkv

import (
	"bytes"
	"log"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"6.5840/labgob"
	"6.5840/labrpc"
	"6.5840/raft"
	"6.5840/shardctrler"
)

const Debug = false

func DPrintf(format string, a ...interface{}) (n int, err error) {
	if Debug {
		log.Printf(format, a...)
	}
	return
}

// Test: concurrent configuration change and restart...
// panic: test timed out after 10m0s
// running tests:
//
//	TestConcurrent3 (9m25s)

// --- FAIL: TestConcurrent3 (169.43s)
//     config.go:75: test took longer than 120 seconds

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

type ShardKV struct {
	mu           sync.Mutex
	me           int
	rf           *raft.Raft
	applyCh      chan raft.ApplyMsg
	make_end     func(string) *labrpc.ClientEnd
	gid          int
	ctrlers      []*labrpc.ClientEnd
	maxraftstate int // snapshot if log grows this big

	// Your definitions here.
	dead              int32
	manager           *shardctrler.Clerk        // ShardCtrler 对应的管理者
	currentCongig     shardctrler.Config        // 保存当前配置信息
	lastConfig        shardctrler.Config        // 保存上一次的配置信息
	shards            map[int]*Shard            // 状态机, 保存分片Id 到 分片数据的映射
	waitChMap         map[int]chan *CommonReply // 通知 chan, key 为日志的下标，值为通道
	LastRequestMap    map[int64]int64           // 保存每个客户端对应的最近一次请求的Id
	persister         *raft.Persister           // 持久化
	lastIncludedIndex int
}

// type ShardState int

// const (
// 	Serving   ShardState = iota // 当前分片正常服务中
// 	Pulling                     // 当前分片正在从其它复制组中拉取信息
// 	BePulling                   // 当前分片正在复制给其它复制组
// 	GCing                       // 当前分片正在等待清楚 （监视器检测到后需要从拥有这个分片的复制组中删除分片）
// )

type ShardState string

const (
	Serving   = "Serving"   // 当前分片正常服务中
	Pulling   = "Pulling"   // 当前分片正在从其它复制组中拉取信息
	BePulling = "BePulling" // 当前分片正在复制给其它复制组
	GCing     = "GCing"     // 当前分片正在等待清楚 （监视器检测到后需要从拥有这个分片的复制组中删除分片）
)

// func (state ShardState) String() string {
// 	switch state {
// 	case Serving:
// 		return "Serving"
// 	case Pulling:
// 		return "Pulling"
// 	case BePulling:
// 		return "BePulling"
// 	case GCing:
// 		return "GCing"
// 	}
// 	return ""
// }

type Shard struct {
	ShardKVDB map[string]string
	State     ShardState
}

func (s *Shard) get(key string) string {
	return s.ShardKVDB[key]
}

func (s *Shard) put(key string, value string) {
	s.ShardKVDB[key] = value
}

func (s *Shard) append(key string, value string) {
	str := s.ShardKVDB[key]
	s.ShardKVDB[key] = str + value
}

// func (s *Shard) hasKey(key string) bool {
// 	_, ok := s.Data[key]
// 	return ok
// }

func MakeShard(state ShardState) *Shard {
	return &Shard{
		ShardKVDB: make(map[string]string),
		State:     state,
	}
}

func (s *Shard) CopyShard() Shard {
	newData := make(map[string]string, len(s.ShardKVDB))
	for k, v := range s.ShardKVDB {
		newData[k] = v
	}

	return Shard{
		ShardKVDB: newData,
		State:     Serving,
	}
}

func mapToString(umap map[string]string) string {
	str := " "
	keys := []string{}
	for k := range umap {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		str += key + ": " + umap[key] + ", "
	}
	str += " "
	return str
}

func ToString(shards map[int]*Shard) string {
	str := ""
	for i := 0; i < shardctrler.NShards; i++ {
		// str += strconv.Itoa(i) + ": " + mapToString(shards[i].ShardKVDB) + string(shards[i].State) + ", "
		str += strconv.Itoa(i) + ": " + string(shards[i].State) + ", "
	}
	return str
}

// 检查当前组是否仍负责管理该分片
// 其它流程同 3A
func (kv *ShardKV) Get(args *GetPutAppendArgs, reply *GetReply) {
	// Your code here.
	kv.mu.Lock()
	// id, gid := kv.me, kv.gid
	// DPrintf("server [%d, %d] receives Get RPC, args [%+v]", id, gid, args)

	// 检查是否由自己负责
	shardId := key2shard(args.Key)
	if !kv.checkShardAndState(shardId) {
		// DPrintf("server [%d, %d] is not serve key [%s] any more, it's shardId [%d]", id, gid, args.Key, shardId)
		reply.Err = ErrWrongGroup
		kv.mu.Unlock()
		return
	}
	kv.mu.Unlock()

	command := Command{
		CommandType: Get,
		Data:        *args,
	}
	response := &CommonReply{Err: OK}
	kv.StartCommand(command, response)
	reply.Value, reply.Err = response.Value, response.Err
	// index, term, isLeader := kv.rf.Start(command)
	// if !isLeader {
	// 	reply.Err = ErrWrongLeader
	// 	return
	// }
	// DPrintf("[%d] send Get to leader, log index [%d], log term [%d], args [%v]", id, index, term, args)
	// kv.mu.Lock()
	// waitChan, exist := kv.waitChMap[index]
	// if !exist {
	// 	kv.waitChMap[index] = make(chan *CommonReply, 1)
	// 	waitChan = kv.waitChMap[index]
	// }
	// DPrintf("[%d] wait for timeout", id)
	// kv.mu.Unlock()
	// select {
	// case res := <-waitChan:
	// 	DPrintf("[%d] receive res from waitChan [%v]", id, res)
	// 	// reply.Value, reply.Err = res.Value, OK
	// 	reply.Value, reply.Err = res.Value, res.Err
	// 	currentTerm, stillLeader := kv.rf.GetState()
	// 	if !stillLeader || currentTerm != term {
	// 		DPrintf("[%d] has accident, stillLeader [%t], term [%d], currentTerm [%d]", id, stillLeader, term, currentTerm)
	// 		reply.Err = ErrWrongLeader
	// 	}
	// case <-time.After(ExecuteTimeout):
	// 	DPrintf("[%d] timeout!", id)
	// 	reply.Err = ErrWrongLeader
	// }
	// kv.mu.Lock()
	// delete(kv.waitChMap, index)
	// kv.mu.Unlock()
}

// 检查当前组是否仍负责管理该分片
// Put 或 Append 还需要先检查是否为过期的 RPC 或已经执行过的命令，避免重复执行
func (kv *ShardKV) PutAppend(args *GetPutAppendArgs, reply *PutAppendReply) {
	// Your code here.
	kv.mu.Lock()
	// id, gid := kv.me, kv.gid
	// DPrintf("server [%d, %d] receives PutAppend RPC, args [%+v]", id, gid, args)

	// 检查是否由自己负责
	shardId := key2shard(args.Key)
	if !kv.checkShardAndState(shardId) {
		// DPrintf("server [%d, %d] is not serve key [%s] any more, it's shardId [%d]", id, gid, args.Key, shardId)
		reply.Err = ErrWrongGroup
		kv.mu.Unlock()
		return
	}
	if kv.isInvalidRequest(args.ClientId, args.RequestId) {
		// DPrintf("server [%d, %d] receives out of data request", id, gid)
		reply.Err = OK
		kv.mu.Unlock()
		return
	}
	kv.mu.Unlock()

	// op := Op{
	// 	ClientId:  args.ClientId,
	// 	RequestId: args.RequestId,
	// 	OpType:    args.Op,
	// 	Key:       args.Key,
	// 	Value:     args.Value,
	// }
	command := Command{
		CommandType: args.OpType,
		Data:        *args,
	}
	response := &CommonReply{Err: OK}
	kv.StartCommand(command, response)
	reply.Err = response.Err
	// index, term, isLeader := kv.rf.Start(command)
	// if !isLeader {
	// 	reply.Err = ErrWrongLeader
	// 	return
	// }
	// kv.mu.Lock()
	// DPrintf("[%d] send PutAppend to leader, log index [%d], log term [%d], args [%v]", id, index, term, args)
	// waitChan, exist := kv.waitChMap[index]
	// if !exist {
	// 	kv.waitChMap[index] = make(chan *CommonReply, 1)
	// 	waitChan = kv.waitChMap[index]
	// }
	// DPrintf("[%d] wait for timeout", id)
	// kv.mu.Unlock()
	// select {
	// case res := <-waitChan:
	// 	DPrintf("[%d] receive res from notifyChan [%v]", id, res)
	// 	reply.Err = res.Err
	// 	currentTerm, stillLeader := kv.rf.GetState()
	// 	if !stillLeader || currentTerm != term {
	// 		DPrintf("[%d] has accident, stillLeader [%t], term [%d], currentTerm [%d]", id, stillLeader, term, currentTerm)
	// 		reply.Err = ErrWrongLeader
	// 	}
	// case <-time.After(ExecuteTimeout):
	// 	DPrintf("[%d] timeout!", id)
	// 	reply.Err = ErrWrongLeader
	// }
	// kv.mu.Lock()
	// delete(kv.waitChMap, index)
	// kv.mu.Unlock()
}

func checkNotPutGetAppend(cmdType string) bool {
	if cmdType != Get && cmdType != Put && cmdType != Append {
		return true
	}
	return false
}

// 提交命令并等待回复或超时
func (kv *ShardKV) StartCommand(command Command, response *CommonReply) {
	index, term, isLeader := kv.rf.Start(command)
	if !isLeader {
		response.Err = ErrWrongLeader
		return
	}
	kv.mu.Lock()
	id, gid := kv.me, kv.gid
	if checkNotPutGetAppend(command.CommandType) {
		DPrintf("server [%d, %d] send command to leader, log index [%d], log term [%d], command [%+v]",
			id, gid, index, term, command)
	}
	waitChan, exist := kv.waitChMap[index]
	if !exist {
		kv.waitChMap[index] = make(chan *CommonReply, 1)
		waitChan = kv.waitChMap[index]
	}
	kv.mu.Unlock()

	select {
	case res := <-waitChan:
		if checkNotPutGetAppend(command.CommandType) {
			DPrintf("server [%d, %d] receive res from waitChan [%+v]", id, gid, res)
		}
		response.Value, response.Err = res.Value, res.Err
		currentTerm, stillLeader := kv.rf.GetState()
		if !stillLeader || currentTerm != term {
			// DPrintf("server [%d, %d] has accident, stillLeader [%t], term [%d], currentTerm [%d]", id, gid, stillLeader, term, currentTerm)
			response.Err = ErrWrongLeader
		}
	case <-time.After(ExecuteTimeout):
		// DPrintf("server [%d, %d] timeout!", id, gid)
		response.Err = ErrWrongLeader
	}

	kv.mu.Lock()
	delete(kv.waitChMap, index)
	kv.mu.Unlock()
}

// the tester calls Kill() when a ShardKV instance won't
// be needed again. you are not required to do anything
// in Kill(), but it might be convenient to (for example)
// turn off debug output from this instance.
func (kv *ShardKV) Kill() {
	kv.rf.Kill()
	// Your code here, if desired.
	atomic.StoreInt32(&kv.dead, 1)
}

func (kv *ShardKV) killed() bool {
	z := atomic.LoadInt32(&kv.dead)
	return z == 1
}

// 需要持久化的字段为 数据库
// 为了避免重复执行命令，每个客户端最近一次请求的 Id 也需要持久化处理
func (kv *ShardKV) encodeState() []byte {
	w := new(bytes.Buffer)
	e := labgob.NewEncoder(w)
	e.Encode(kv.shards)
	e.Encode(kv.LastRequestMap)
	e.Encode(kv.lastConfig)
	e.Encode(kv.currentCongig)
	kvstate := w.Bytes()
	return kvstate
}

// 读取持久化状态, 仿照 Raft 里面的写就可以了
func (kv *ShardKV) readPersist(data []byte) {
	if data == nil || len(data) < 1 { // bootstrap without any state?
		return
	}
	r := bytes.NewBuffer(data)
	d := labgob.NewDecoder(r)
	shards := map[int]*Shard{}
	lastRequestMap := map[int64]int64{}
	lastConfig := shardctrler.Config{}
	currentCongig := shardctrler.Config{}
	if d.Decode(&shards) != nil || d.Decode(&lastRequestMap) != nil ||
		d.Decode(&lastConfig) != nil || d.Decode(&currentCongig) != nil {
		return
	} else {
		kv.shards = shards
		kv.LastRequestMap = lastRequestMap
		kv.lastConfig = lastConfig
		kv.currentCongig = currentCongig
	}
}

// 监听 Raft 提交的 applyMsg, 根据 applyMsg 的类别执行不同的操作
// 为命令的话，必执行，执行完后检查是否需要给 waitCh 发通知
// 为快照的话读快照，更新状态
func (kv *ShardKV) applier() {
	for !kv.killed() {
		applyMsg := <-kv.applyCh
		kv.mu.Lock()
		id, gid := kv.me, kv.gid
		// DPrintf("server [%d, %d] in applier() receives applyMsg [%+v]", id, gid, applyMsg)
		// 根据收到的是命令还是快照来决定相应的操作，3A仅需处理命令
		if applyMsg.CommandValid {
			// 3B 需要判断日志是否被裁剪了
			if applyMsg.CommandIndex <= kv.lastIncludedIndex {
				// DPrintf("server [%d, %d] has snapshot this command!", id, gid)
				kv.mu.Unlock()
				continue
			}
			DPrintf("server [%d, %d] in applier() receives applyMsg [%+v]", id, gid, applyMsg)

			// op := applyMsg.Command.(Op)
			// kv.execute(&op)

			reply := kv.execute(applyMsg.Command)
			currentTerm, isLeader := kv.rf.GetState()
			// 若当前服务器已经不再是 leader 或者是旧 leader，不需要通知回复客户端
			// 指南中提到的情况：Clerk 在一个任期内向 kvserver 领导者发送请求, 可能在此期间当前领导者丧失了领导地位但是又重新当选了 Leader
			// 虽然它还是 Leader, 但是已经不能在进行回复了，需要满足线性一致性 (可能客户端发起 Get 时应该获取的结果是 0, 但是在次期间增加了 1。若现在回复的话会回复 1, 但是根据请求时间来看应该返回 0)
			// 所以不给客户端响应, 让其超时, 然后重新发送 Get, 此时的 Get 得到的结果就应该是 1 了 (只要任期没变, 都是同一个 Leader 在处理的话, 因为有重复命令的检查, 必定满足线性一致性)
			if isLeader && applyMsg.CommandTerm == currentTerm {
				kv.notifyWaitCh(applyMsg.CommandIndex, reply)
			}

			// 3B 执行完命令后检查状态, 有必要的化执行快照压缩 Raft 的日志
			if kv.maxraftstate != -1 && kv.persister.RaftStateSize() >= kv.maxraftstate {
				// DPrintf("server [%d, %d] has too big RaftStateSize, run Raft.Snapshot()", id, gid)
				kv.rf.Snapshot(applyMsg.CommandIndex, kv.encodeState())
			}
			kv.lastIncludedIndex = applyMsg.CommandIndex

		} else if applyMsg.SnapshotValid {
			// 一定是 Follower 收到了快照, 若是最新的快照, 读取快照信息并更新自身状态
			if applyMsg.SnapshotIndex <= kv.lastIncludedIndex {
				// DPrintf("server [%d, %d] receive old snapshot, lastIncludeIndex [%d], applyMsg.SnapshotIndex [%d]",
				// 	id, gid, kv.lastIncludedIndex, applyMsg.SnapshotIndex)
				kv.mu.Unlock()
				continue
			}
			kv.readPersist(applyMsg.Snapshot)
			kv.lastIncludedIndex = applyMsg.SnapshotIndex
		}
		kv.mu.Unlock()
	}
}

// 根据 OpType 选择操作执行
func (kv *ShardKV) execute(cmd interface{}) *CommonReply {
	// DPrintf("[%d] apply command [%v]", kv.me, op)
	command := cmd.(Command)

	// 注意这个 reply, 其它情况目前并没有专门的 Err

	reply := &CommonReply{
		Err: OK,
	}
	// DPrintf("server [%d, %d] ready for execute command [%+v]", kv.me, kv.gid, command)
	switch command.CommandType {
	case Get, Put, Append:
		op := command.Data.(GetPutAppendArgs)
		kv.processGetPutAppend(&op, reply)
	case AddConfig:
		config := command.Data.(shardctrler.Config)
		kv.processAddConfig(&config, reply)
	case InsertShard:
		response := command.Data.(PullShardReply)
		kv.processInsertShard(&response, reply)
	case DeleteShard:
		args := command.Data.(RemoveShardArgs)
		kv.processDeleteShard(&args, reply)
	case AdjustShardState:
		args := command.Data.(AdjustShardArgs)
		kv.adjustGCingShard(&args, reply)
	}
	return reply
}

// 先检查此时该 key 是否仍由自己负责
// 执行命令，若为重复命令并且不是 Get 的话，直接返回即可
// 否则根据 OpType 执行命令并更新该客户端最近一次请求的Id
func (kv *ShardKV) processGetPutAppend(op *GetPutAppendArgs, reply *CommonReply) {
	shardId := key2shard(op.Key)
	// DPrintf("server [%d, %d] processGetPutAppend op [%+v], shardId [%d]", kv.me, kv.gid, *op, shardId)
	if !kv.checkShardAndState(shardId) {
		// DPrintf("server [%d, %d] receive wrong group processGetPutAppend request", kv.me, kv.gid)
		reply.Err = ErrWrongGroup
		return
	}
	if op.OpType == Get || !kv.isInvalidRequest(op.ClientId, op.RequestId) {
		switch op.OpType {
		case Get:
			reply.Value = kv.shards[shardId].get(op.Key)
		case Put:
			kv.shards[shardId].put(op.Key, op.Value)
		case Append:
			kv.shards[shardId].append(op.Key, op.Value)
		}
		// DPrintf("server [%d, %d] processGetPutAppend OpType [%s] success!", kv.me, kv.gid, op.OpType)
		kv.UpdateLastRequest(op)
	}
}

// 检查是否是真的最新的配置 (配置号应该比自己的更大一号)
// 检查所有分片的所属情况，若需要从其它副本组拉去分片信息或者给其它副本组发送分片信息的话修改分片状态
func (kv *ShardKV) processAddConfig(newConfig *shardctrler.Config, reply *CommonReply) {
	DPrintf("server [%d, %d] processAddConfig, currentCongig [%+v], newConfig [%+v]", kv.me, kv.gid, kv.currentCongig, *newConfig)
	if newConfig.Num == kv.currentCongig.Num+1 {
		states := ""
		for i := 0; i < shardctrler.NShards; i++ {
			// 第 i 个分片从不由自己管理到由自己管理
			if newConfig.Shards[i] == kv.gid && kv.currentCongig.Shards[i] != kv.gid {
				// 若当前该分片由其它组管理的话，需要从其它组那里拉去信息
				if kv.currentCongig.Shards[i] != 0 {
					kv.shards[i].State = Pulling
				}
			}
			// 第 i 个分片从由自己管理到不由自己管理
			if newConfig.Shards[i] != kv.gid && kv.currentCongig.Shards[i] == kv.gid {
				// 若当前分片需要给其它组的话，设置状态为待拉取
				if newConfig.Shards[i] != 0 {
					kv.shards[i].State = BePulling
				}
			}
			states += string(kv.shards[i].State) + ", "
		}
		kv.lastConfig = kv.currentCongig
		kv.currentCongig = *newConfig
		DPrintf("server [%d, %d] updates shards state and config over, shard state [%s]", kv.me, kv.gid, states)

		reply.Err = OK
	} else {
		// 同理,该设置 reply.Err 为什么
		reply.Err = OtherErr
		// DPrintf("server [%d, %d] receive old processAddConfig request", kv.me, kv.gid)
	}
}

// 先判断配置号是否相等
// 依次更新每个分片中维护的键值数据库
// 更新每个分片对应的客户端最近一次请求的Id, 避免重复执行命令
func (kv *ShardKV) processInsertShard(response *PullShardReply, reply *CommonReply) {
	DPrintf("server [%d, %d] processInsertShard, currentCongigNum [%d], response.ConfigNum [%d]", kv.me, kv.gid, kv.currentCongig.Num, response.ConfigNum)

	if response.ConfigNum == kv.currentCongig.Num {
		Shards := response.Shards
		for shardId, shard := range Shards {
			// 深拷贝？？
			oldShard := kv.shards[shardId]
			if oldShard.State == Pulling {
				for key, value := range shard.ShardKVDB {
					oldShard.ShardKVDB[key] = value
				}
				oldShard.State = GCing
			}
		}
		DPrintf("server [%d, %d] updates shards [%+v], now shards [%s]", kv.me, kv.gid, Shards, ToString(kv.shards))
		LastRequestMap := response.LastRequestMap
		// DPrintf("server [%d, %d] kv.LastRequestMap [%+v], response.LastRequestMap [%+v]", kv.me, kv.gid, kv.LastRequestMap, LastRequestMap)
		for clientId, requestId := range LastRequestMap {
			kv.LastRequestMap[clientId] = max(requestId, kv.LastRequestMap[clientId])
		}
		// DPrintf("server [%d, %d] updates LastRequestMap [%+v]", kv.me, kv.gid, kv.LastRequestMap)
		DPrintf("server [%d, %d] InsertShard over", kv.me, kv.gid)
	} else {
		// 同理,该设置 reply.Err 为什么
		reply.Err = OtherErr

		// DPrintf("server [%d, %d] receive old insert shards request", kv.me, kv.gid)
	}
}

// 先判断配置号是否相等
// 若当前分片确实原本由自己负责并且状态为待拉取状态的话就重置该分片
func (kv *ShardKV) processDeleteShard(args *RemoveShardArgs, reply *CommonReply) {
	DPrintf("server [%d, %d] processDeleteShard, currentCongigNum [%d], args.ConfigNum [%d]", kv.me, kv.gid, kv.currentCongig.Num, args.ConfigNum)

	if args.ConfigNum == kv.currentCongig.Num {
		DPrintf("server [%d, %d] original shards [%s]", kv.me, kv.gid, ToString(kv.shards))
		for _, shardId := range args.ShardIds {
			_, ok := kv.shards[shardId]
			if ok && kv.shards[shardId].State == BePulling {
				kv.shards[shardId] = MakeShard(Serving)
			}
		}
		DPrintf("server [%d, %d] updates shards [%s]", kv.me, kv.gid, ToString(kv.shards))
	} else {
		// 同理,该设置 reply.Err 为什么 (后面需要根据返回值来确定是否更改状态为 Serving, 所以必须返回非 OK)
		// reply.Err = ErrWrongLeader
		reply.Err = OtherErr
		if args.ConfigNum < kv.currentCongig.Num {
			reply.Err = OK
		}
		DPrintf("server [%d, %d] receive old delete shards request", kv.me, kv.gid)
	}
}

func (kv *ShardKV) adjustGCingShard(args *AdjustShardArgs, reply *CommonReply) {
	DPrintf("server [%d, %d] adjustGCingShard, currentCongigNum [%d], args.ConfigNum [%d]", kv.me, kv.gid, kv.currentCongig.Num, args.ConfigNum)

	if args.ConfigNum == kv.currentCongig.Num {
		DPrintf("server [%d, %d] original shards [%s]", kv.me, kv.gid, ToString(kv.shards))
		// for _, shardId := range args.ShardIds {
		// 	_, ok := kv.shards[shardId]
		// 	if ok && kv.shards[shardId].State == BePulling {
		// 		kv.shards[shardId] = MakeShard(Serving)
		// 	}
		// }
		for _, shardId := range args.ShardIds {
			// 同理, 按理来说必定存在
			if _, ok := kv.shards[shardId]; ok {
				kv.shards[shardId].State = Serving
			}
		}
		DPrintf("server [%d, %d] updates shards [%s]", kv.me, kv.gid, ToString(kv.shards))
	} else {
		// 同理,该设置 reply.Err 为什么 (后面需要根据返回值来确定是否更改状态为 Serving, 所以必须返回非 OK)
		// reply.Err = ErrWrongLeader
		reply.Err = OtherErr
		if args.ConfigNum < kv.currentCongig.Num {
			reply.Err = OK
		}
		DPrintf("server [%d, %d] receive old delete shards request", kv.me, kv.gid)
	}
}

// 给 waitCh 发送通知，让其生成响应。必须在发送前检查一下 waitCh 是否关闭
func (kv *ShardKV) notifyWaitCh(index int, reply *CommonReply) {
	if waitCh, ok := kv.waitChMap[index]; ok {
		// DPrintf("server [%d, %d] notifyWaitCh [%d]", kv.me, kv.gid, index)
		waitCh <- reply
	}
}

// 检查当前分片是否仍由自己负责并且当前分片的状态正常
func (kv *ShardKV) checkShardAndState(shardId int) bool {
	shard, ok := kv.shards[shardId]
	// if ok {
	// 	DPrintf("server [%d, %d] shard [%d] gid [%d] kv.currentCongig.Shards[shardId] [%d] shard state [%v]",
	// 		kv.me, kv.gid, shardId, kv.gid, kv.currentCongig.Shards[shardId], shard.State)
	// }
	if ok && kv.currentCongig.Shards[shardId] == kv.gid &&
		(shard.State == Serving || shard.State == GCing) {
		return true
	}
	// DPrintf("server [%d, %d] is not responsible for shardId [%d] or the current shard is changing", kv.me, kv.gid, shardId)
	// DPrintf("server [%d, %d] currentConfig [%+v], shard state [%s]", kv.me, kv.gid, kv.currentCongig, shard.State)
	return false
}

// 检查当前命令是否为无效的命令 (非 Get 命令需要检查，可能为过期的 RPC 或已经执行过的命令)
func (kv *ShardKV) isInvalidRequest(clientId int64, requestId int64) bool {
	if lastRequestId, ok := kv.LastRequestMap[clientId]; ok {
		if requestId <= lastRequestId {
			// DPrintf("server [%d, %d] receives out of data request [%d] from clientId [%d]", kv.me, kv.gid, requestId, clientId)
			return true
		}
	}
	return false
}

// 更新对应客户端对应的最近一次请求的Id, 这样可以避免今后执行过期的 RPC 或已经执行过的命令
func (kv *ShardKV) UpdateLastRequest(op *GetPutAppendArgs) {
	lastRequestId, ok := kv.LastRequestMap[op.ClientId]
	if (ok && lastRequestId < op.RequestId) || !ok {
		// DPrintf("server [%d, %d] update LastRequest of client[%d] from [%d] to [%d]",
		// 	kv.me, kv.gid, op.ClientId, lastRequestId, op.RequestId)
		kv.LastRequestMap[op.ClientId] = op.RequestId
	}
}

// Leader 节点需定期向 ShardCtrler 询问最新配置信息
// 若当前正在更改自身配置则放弃本次询问
// 若发现获取的配置为新配置则更新自身配置
func (kv *ShardKV) monitorRequestConfig() {
	for !kv.killed() {
		if _, isLeader := kv.rf.GetState(); !isLeader {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		kv.mu.Lock()
		id, gid := kv.me, kv.gid
		// 检查当前是否没有分片信息正在更改
		isProcessShardCommand := false
		for _, shard := range kv.shards {
			if shard.State != Serving {
				isProcessShardCommand = true
				break
			}
		}
		currentConfigNum := kv.currentCongig.Num
		DPrintf("server [%d, %d] in monitorRequestConfig() isProcessShardCommand, %t, currentConfig [%+v]",
			id, gid, isProcessShardCommand, kv.currentCongig)
		if isProcessShardCommand {
			DPrintf("server [%d, %d] shards [%s]", id, gid, ToString(kv.shards))
		}
		kv.mu.Unlock()

		if !isProcessShardCommand {
			// 注意这里不能请求最新的
			// newConfig := kv.manager.Query(-1)
			newConfig := kv.manager.Query(currentConfigNum + 1)
			// DPrintf("server [%d, %d] Query config, newConfig [%+v]", id, gid, newConfig)
			if newConfig.Num == currentConfigNum+1 {
				reply := &CommonReply{}
				DPrintf("server [%d, %d] receive new Config [%+v], process add config", id, gid, newConfig)
				command := Command{
					CommandType: AddConfig,
					Data:        newConfig,
				}
				kv.StartCommand(command, reply)
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// 定期检查是否需要向其它副本组拉取分片
// 若确定了需要向某个副本组拉取分片的话, 调用 RPC 获取回复 (包含分片以及每个分片对应的客户端最近一次的请求信息)
// 得到这些信息后, 给 Raft 提交命令等待达成共识并执行
func (kv *ShardKV) monitorInsert() {
	for !kv.killed() {
		if _, isLeader := kv.rf.GetState(); !isLeader {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		kv.mu.Lock()
		id, gid := kv.me, kv.gid
		// 获取需要拉取的分片所在的组并将分片按组整合 (key 为 oldGid, Vaule 为 shardId[])
		// 这样可以一次直接拉取某个组所有符合要求的分片
		groups := kv.getShardIdsWithSpecifiedState(Pulling)
		wg := &sync.WaitGroup{}
		wg.Add(len(groups))
		if len(groups) != 0 {
			DPrintf("server [%d, %d] in monitorInsert() requests pull shards in monitorInsert, groups [%+v]", id, gid, groups)
		}
		for oldGid, shardIds := range groups {
			configNum, servers := kv.currentCongig.Num, kv.lastConfig.Groups[oldGid]
			// 增加配置号以避免更改配置的请求被重复执行
			go func(oldGid int, configNum int, servers []string, shardIds []int) {
				defer wg.Done()
				// 找 Leader
				DPrintf("server [%d, %d] send GetShards request to other servers, oldGid [%d], configNum [%d], servers [%+v], shardIds [%+v]",
					id, gid, oldGid, configNum, servers, shardIds)
				for _, server := range servers {
					args := &PullShardArgs{
						Gid:       oldGid,
						ShardIds:  shardIds,
						ConfigNum: configNum,
					}
					reply := &PullShardReply{}
					srv := kv.make_end(server)
					ok := srv.Call("ShardKV.GetShards", args, reply)
					if ok && reply.Err == OK {
						reply.ConfigNum = configNum
						command := Command{
							CommandType: InsertShard,
							Data:        *reply,
						}
						DPrintf("server [%d, %d] StartCommand [%+v]", id, gid, command)
						kv.StartCommand(command, &CommonReply{})
					}
				}
			}(oldGid, configNum, servers, shardIds)
		}
		kv.mu.Unlock()
		wg.Wait()
		// DPrintf("[%d, %d] get shards end!", id, gid)
		time.Sleep(100 * time.Millisecond)
	}
}

// 返回需要的分片数据
// 只有 Leader 能进行回复
func (kv *ShardKV) GetShards(args *PullShardArgs, reply *PullShardReply) {
	if _, isLeader := kv.rf.GetState(); !isLeader {
		reply.Err = ErrWrongLeader
		return
	}
	kv.mu.Lock()
	defer kv.mu.Unlock()
	// 判断是否是合法的请求
	if args.ConfigNum == kv.currentCongig.Num {
		shards := make(map[int]Shard)
		for _, shardId := range args.ShardIds {
			// 感觉这个没必要, 必定存在
			_, ok := kv.shards[shardId]
			if ok && kv.shards[shardId].State == BePulling {
				shards[shardId] = kv.shards[shardId].CopyShard()
			}
		}
		reply.Err, reply.Shards, reply.LastRequestMap = OK, shards, kv.copyLastRequestMap()
	} else {
		DPrintf("server [%d, %d] receives out of data GetShards request, currentConfigNum [%d], requestConfigNum [%d]", kv.me, kv.gid, kv.currentCongig.Num, args.ConfigNum)
		reply.Err = ErrWrongGroup
	}
}

func (kv *ShardKV) copyLastRequestMap() map[int64]int64 {
	lastRequestMap := make(map[int64]int64)
	for clientId, requestId := range kv.LastRequestMap {
		lastRequestMap[clientId] = requestId
	}
	return lastRequestMap
}

// 获取处于指定状态的分片之前所属的复制组, 得到复制组Id 到分片Id 的映射
func (kv *ShardKV) getShardIdsWithSpecifiedState(state ShardState) map[int][]int {
	tmp := make(map[int][]int)
	// 当前复制组向其它复制组拉去分片 或 当前复制组拉取完分片后通知其它复制组删除分片
	// 拉取分片以及请求其它组删除分配的请求复制组都在上一个配置中, 所以应检查上一个配置
	for shardId, shard := range kv.shards {
		if shard.State == state {
			gid := kv.lastConfig.Shards[shardId]
			if _, ok := tmp[gid]; !ok {
				tmp[gid] = make([]int, 0)
			}
			tmp[gid] = append(tmp[gid], shardId)
		}
	}
	return tmp
}

func (kv *ShardKV) monitorGC() {
	for !kv.killed() {
		if _, isLeader := kv.rf.GetState(); !isLeader {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		kv.mu.Lock()
		id, gid := kv.me, kv.gid
		// 获取需要拉取的分片所在的组并将分片按组整合 (key 为 oldGid, Vaule 为 shardId[])
		// 这样可以一次直接拉取某个组所有符合要求的分片
		groups := kv.getShardIdsWithSpecifiedState(GCing)
		wg := &sync.WaitGroup{}
		wg.Add(len(groups))
		if len(groups) != 0 {
			DPrintf("server [%d, %d] in monitorGC() is requested pull shards in monitorGC, groups [%+v]", id, gid, groups)
		}
		for oldGid, shardIds := range groups {
			configNum, servers := kv.currentCongig.Num, kv.lastConfig.Groups[oldGid]
			// 增加配置号以避免更改配置的请求被重复执行
			go func(oldGid int, configNum int, servers []string, shardIds []int) {
				defer wg.Done()
				// 找 Leader
				DPrintf("server [%d, %d] send DeleteShards request, oldGid [%d], configNum [%d], servers [%+v], shardIds [%+v]",
					id, gid, oldGid, configNum, servers, shardIds)
				for _, server := range servers {
					args := &RemoveShardArgs{
						ShardIds:  shardIds,
						ConfigNum: configNum,
					}
					reply := &RemoveShardReply{}
					srv := kv.make_end(server)
					ok := srv.Call("ShardKV.DeleteShards", args, reply)
					if ok && reply.Err == OK {
						adjargs := AdjustShardArgs{
							ShardIds:  shardIds,
							ConfigNum: configNum,
						}
						command := Command{
							CommandType: AdjustShardState,
							Data:        adjargs,
						}
						kv.StartCommand(command, &CommonReply{})
						DPrintf("server [%d, %d] adjust shard state over!!", id, gid)
						// kv.mu.Lock()
						// for _, shardId := range shardIds {
						// 	// 同理, 按理来说必定存在
						// 	if _, ok := kv.shards[shardId]; ok {
						// 		kv.shards[shardId].State = Serving
						// 	}
						// }
						// DPrintf("server [%d, %d] adjust shard state over!!, shards [%s]", id, gid, ToString(kv.shards))
						// kv.mu.Unlock()
					}
				}
			}(oldGid, configNum, servers, shardIds)
		}
		kv.mu.Unlock()
		wg.Wait()
		time.Sleep(100 * time.Millisecond)
	}
}

func (kv *ShardKV) DeleteShards(args *RemoveShardArgs, reply *RemoveShardReply) {
	command := Command{
		CommandType: DeleteShard,
		Data:        *args,
	}
	response := &CommonReply{}
	DPrintf("server [%d, %d] StartCommand [%+v]", kv.me, kv.gid, command)
	kv.StartCommand(command, response)
	reply.Err = response.Err
}

func (kv *ShardKV) monitorBePulling() {
	for !kv.killed() {
		if _, isLeader := kv.rf.GetState(); !isLeader {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		kv.mu.Lock()
		id, gid := kv.me, kv.gid
		groups := make(map[int][]int)
		for shardId, shard := range kv.shards {
			if shard.State == BePulling {
				gid := kv.currentCongig.Shards[shardId]
				if _, ok := groups[gid]; !ok {
					groups[gid] = make([]int, 0)
				}
				groups[gid] = append(groups[gid], shardId)
			}
		}
		wg := &sync.WaitGroup{}
		wg.Add(len(groups))
		if len(groups) != 0 {
			DPrintf("server [%d, %d] in monitorBePulling() requests bepulled shards in monitorBePulling, groups [%+v]", id, gid, groups)
		}
		for oldGid, shardIds := range groups {
			configNum, servers := kv.currentCongig.Num, kv.lastConfig.Groups[oldGid]
			// 增加配置号以避免更改配置的请求被重复执行
			go func(oldGid int, configNum int, servers []string, shardIds []int) {
				defer wg.Done()
				// 找 Leader
				DPrintf("server [%d, %d] send GetShards request to other servers, oldGid [%d], configNum [%d], servers [%+v], shardIds [%+v]",
					id, gid, oldGid, configNum, servers, shardIds)
				for _, server := range servers {
					args := &CheckArgs{
						ShardIds:  shardIds,
						ConfigNum: configNum,
					}
					reply := &CheckReply{}
					srv := kv.make_end(server)
					ok := srv.Call("ShardKV.CheckShards", args, reply)
					if ok && reply.Err == OK {
						adjargs := AdjustShardArgs{
							ShardIds:  shardIds,
							ConfigNum: configNum,
						}
						command := Command{
							CommandType: AdjustShardState,
							Data:        adjargs,
						}
						kv.StartCommand(command, &CommonReply{})
						DPrintf("server [%d, %d] adjust shard state over!!", id, gid)
					}
				}
			}(oldGid, configNum, servers, shardIds)
		}
		kv.mu.Unlock()
		wg.Wait()
		// DPrintf("[%d, %d] get shards end!", id, gid)
		time.Sleep(100 * time.Millisecond)
	}
}

func (kv *ShardKV) CheckShards(args *CheckArgs, reply *CheckReply) {
	kv.mu.Lock()
	if kv.currentCongig.Num > args.ConfigNum {
		reply.Err = OK
	} else {
		reply.Err = OtherErr
	}
	kv.mu.Unlock()
}

// servers[] contains the ports of the servers in this group.
//
// me is the index of the current server in servers[].
//
// the k/v server should store snapshots through the underlying Raft
// implementation, which should call persister.SaveStateAndSnapshot() to
// atomically save the Raft state along with the snapshot.
//
// the k/v server should snapshot when Raft's saved state exceeds
// maxraftstate bytes, in order to allow Raft to garbage-collect its
// log. if maxraftstate is -1, you don't need to snapshot.
//
// gid is this group's GID, for interacting with the shardctrler.
//
// pass ctrlers[] to shardctrler.MakeClerk() so you can send
// RPCs to the shardctrler.
//
// make_end(servername) turns a server name from a
// Config.Groups[gid][i] into a labrpc.ClientEnd on which you can
// send RPCs. You'll need this to send RPCs to other groups.
//
// look at client.go for examples of how to use ctrlers[]
// and make_end() to send RPCs to the group owning a specific shard.
//
// StartServer() must return quickly, so it should start goroutines
// for any long-running work.
func StartServer(servers []*labrpc.ClientEnd, me int, persister *raft.Persister, maxraftstate int, gid int, ctrlers []*labrpc.ClientEnd, make_end func(string) *labrpc.ClientEnd) *ShardKV {
	// call labgob.Register on structures you want
	// Go's RPC library to marshall/unmarshall.
	labgob.Register(Op{})

	kv := new(ShardKV)
	kv.me = me
	kv.maxraftstate = maxraftstate
	kv.make_end = make_end
	kv.gid = gid
	kv.ctrlers = ctrlers

	// Your initialization code here.
	// Use something like this to talk to the shardctrler:
	// kv.mck = shardctrler.MakeClerk(kv.ctrlers)
	kv.applyCh = make(chan raft.ApplyMsg)
	kv.rf = raft.Make(servers, me, persister, kv.applyCh)
	kv.manager = shardctrler.MakeClerk(kv.ctrlers)
	kv.currentCongig, kv.lastConfig = shardctrler.Config{}, shardctrler.Config{}
	kv.shards = make(map[int]*Shard)
	kv.waitChMap = make(map[int]chan *CommonReply)
	kv.LastRequestMap = make(map[int64]int64)
	kv.persister = persister
	kv.readPersist(kv.persister.ReadSnapshot())

	DPrintf("server [%d, %d] init config [%+v], gid [%d]", me, kv.gid, kv.currentCongig, gid)

	labgob.Register(Command{})
	labgob.Register(shardctrler.Config{})
	labgob.Register(PutAppendArgs{})
	labgob.Register(GetPutAppendArgs{})
	labgob.Register(PullShardReply{})
	labgob.Register(RemoveShardArgs{})
	labgob.Register(AdjustShardArgs{})

	// 初始化所有切片
	for shardId := 0; shardId < shardctrler.NShards; shardId++ {
		if _, ok := kv.shards[shardId]; !ok {
			kv.shards[shardId] = MakeShard(Serving)
		}
	}
	go kv.applier()
	go kv.monitorRequestConfig()
	go kv.monitorInsert()
	go kv.monitorGC()
	go kv.monitorBePulling()
	return kv
}
