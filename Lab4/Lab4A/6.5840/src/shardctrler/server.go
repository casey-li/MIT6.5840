package shardctrler

import (
	"log"
	"math"
	"sort"
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

const (
	Join  = "Join"
	Move  = "Move"
	Leave = "Leave"
	Query = "Query"
)

const ExecuteTimeout = 500 * time.Millisecond

// 同 KVServer, 只不过状态机不再是键值数据库而是 configs
type ShardCtrler struct {
	mu      sync.Mutex
	me      int
	rf      *raft.Raft
	applyCh chan raft.ApplyMsg

	// Your data here.
	dead           int32
	configs        []Config         // indexed by config num
	waitChMap      map[int]chan *Op // 通知 chan, key 为日志的下标，值为通道
	LastRequestMap map[int64]int64  // 保存每个客户端对应的最近一次请求的 Id
}

// 同 KVServer, 只不过将字段从 K/V 换成了执行 Join/Leave/Move/Query Args 所需的参数
type Op struct {
	// Your data here.
	ClientId  int64            // 客户端 Id
	RequestId int64            // 请求 Id
	OpType    string           // 操作类别
	Servers   map[int][]string // Join, new GID -> servers mappings
	GIDs      []int            // Leave
	Shard     int              // Move
	GID       int              // Move
	Num       int              // Query, desired config number
	Configure Config
}

func (sc *ShardCtrler) Join(args *JoinArgs, reply *JoinReply) {
	// Your code here.
	sc.mu.Lock()
	id := sc.me
	if sc.isInvalidRequest(args.ClientId, args.RequestId) {
		DPrintf("[%d] receive out of data request", id)
		reply.Err = OK
		sc.mu.Unlock()
		return
	}
	sc.mu.Unlock()
	op := Op{
		ClientId:  args.ClientId,
		RequestId: args.RequestId,
		OpType:    Join,
		Servers:   args.Servers,
	}
	index, term, isLeader := sc.rf.Start(op)
	if !isLeader {
		reply.WrongLeader = true
		return
	}
	sc.mu.Lock()
	DPrintf("[%d] send Join to leader, log index [%d], log term [%d], args [%v]", id, index, term, args)
	waitChan, exist := sc.waitChMap[index]
	if !exist {
		sc.waitChMap[index] = make(chan *Op, 1)
		waitChan = sc.waitChMap[index]
	}
	sc.mu.Unlock()

	select {
	case res := <-waitChan:
		DPrintf("[%d] receive res from waitChan [%v]", id, res)
		reply.Err = OK
		currentTerm, stillLeader := sc.rf.GetState()
		if !stillLeader || currentTerm != term {
			DPrintf("[%d] has accident, stillLeader [%t], term [%d], currentTerm [%d]", id, stillLeader, term, currentTerm)
			reply.WrongLeader = true
		}
	case <-time.After(ExecuteTimeout):
		DPrintf("[%d] timeout!", id)
		reply.WrongLeader = true
	}

	sc.mu.Lock()
	delete(sc.waitChMap, index)
	sc.mu.Unlock()
}

func (sc *ShardCtrler) Leave(args *LeaveArgs, reply *LeaveReply) {
	// Your code here.
	sc.mu.Lock()
	id := sc.me
	if sc.isInvalidRequest(args.ClientId, args.RequestId) {
		DPrintf("[%d] receive out of data request", id)
		reply.Err = OK
		sc.mu.Unlock()
		return
	}
	sc.mu.Unlock()
	op := Op{
		ClientId:  args.ClientId,
		RequestId: args.RequestId,
		OpType:    Leave,
		GIDs:      args.GIDs,
	}
	index, term, isLeader := sc.rf.Start(op)
	if !isLeader {
		reply.WrongLeader = true
		return
	}
	sc.mu.Lock()
	DPrintf("[%d] send Leave to leader, log index [%d], log term [%d], args [%v]", id, index, term, args)
	waitChan, exist := sc.waitChMap[index]
	if !exist {
		sc.waitChMap[index] = make(chan *Op, 1)
		waitChan = sc.waitChMap[index]
	}
	sc.mu.Unlock()

	select {
	case res := <-waitChan:
		DPrintf("[%d] receive res from waitChan [%v]", id, res)
		reply.Err = OK
		currentTerm, stillLeader := sc.rf.GetState()
		if !stillLeader || currentTerm != term {
			DPrintf("[%d] has accident, stillLeader [%t], term [%d], currentTerm [%d]", id, stillLeader, term, currentTerm)
			reply.WrongLeader = true
		}
	case <-time.After(ExecuteTimeout):
		DPrintf("[%d] timeout!", id)
		reply.WrongLeader = true
	}

	sc.mu.Lock()
	delete(sc.waitChMap, index)
	sc.mu.Unlock()
}

func (sc *ShardCtrler) Move(args *MoveArgs, reply *MoveReply) {
	// Your code here.
	sc.mu.Lock()
	id := sc.me
	if sc.isInvalidRequest(args.ClientId, args.RequestId) {
		DPrintf("[%d] receive out of data request", id)
		reply.Err = OK
		sc.mu.Unlock()
		return
	}
	sc.mu.Unlock()
	op := Op{
		ClientId:  args.ClientId,
		RequestId: args.RequestId,
		OpType:    Move,
		Shard:     args.Shard,
		GID:       args.GID,
	}
	index, term, isLeader := sc.rf.Start(op)
	if !isLeader {
		reply.WrongLeader = true
		return
	}
	sc.mu.Lock()
	DPrintf("[%d] send Move to leader, log index [%d], log term [%d], args [%v]", id, index, term, args)
	waitChan, exist := sc.waitChMap[index]
	if !exist {
		sc.waitChMap[index] = make(chan *Op, 1)
		waitChan = sc.waitChMap[index]
	}
	sc.mu.Unlock()

	select {
	case res := <-waitChan:
		DPrintf("[%d] receive res from waitChan [%v]", id, res)
		reply.Err = OK
		currentTerm, stillLeader := sc.rf.GetState()
		if !stillLeader || currentTerm != term {
			DPrintf("[%d] has accident, stillLeader [%t], term [%d], currentTerm [%d]", id, stillLeader, term, currentTerm)
			reply.WrongLeader = true
		}
	case <-time.After(ExecuteTimeout):
		DPrintf("[%d] timeout!", id)
		reply.WrongLeader = true
	}

	sc.mu.Lock()
	delete(sc.waitChMap, index)
	sc.mu.Unlock()
}

func (sc *ShardCtrler) Query(args *QueryArgs, reply *QueryReply) {
	// Your code here.
	sc.mu.Lock()
	id := sc.me
	if sc.isInvalidRequest(args.ClientId, args.RequestId) {
		DPrintf("[%d] receive out of data request", id)
		reply.Err = OK
		sc.mu.Unlock()
		return
	}
	sc.mu.Unlock()
	op := Op{
		ClientId:  args.ClientId,
		RequestId: args.RequestId,
		OpType:    Query,
		Num:       args.Num,
	}
	index, term, isLeader := sc.rf.Start(op)
	if !isLeader {
		reply.WrongLeader = true
		return
	}
	sc.mu.Lock()
	DPrintf("[%d] send Query to leader, log index [%d], log term [%d], args [%v]", id, index, term, args)
	waitChan, exist := sc.waitChMap[index]
	if !exist {
		sc.waitChMap[index] = make(chan *Op, 1)
		waitChan = sc.waitChMap[index]
	}
	sc.mu.Unlock()

	select {
	case res := <-waitChan:
		DPrintf("[%d] receive res from waitChan [%v]", id, res)
		reply.Config, reply.Err = res.Configure, OK
		currentTerm, stillLeader := sc.rf.GetState()
		if !stillLeader || currentTerm != term {
			DPrintf("[%d] has accident, stillLeader [%t], term [%d], currentTerm [%d]", id, stillLeader, term, currentTerm)
			reply.WrongLeader = true
		}
	case <-time.After(ExecuteTimeout):
		DPrintf("[%d] timeout!", id)
		reply.WrongLeader = true
	}

	sc.mu.Lock()
	delete(sc.waitChMap, index)
	sc.mu.Unlock()
}

// the tester calls Kill() when a ShardCtrler instance won't
// be needed again. you are not required to do anything
// in Kill(), but it might be convenient to (for example)
// turn off debug output from this instance.
func (sc *ShardCtrler) Kill() {
	sc.rf.Kill()
	// Your code here, if desired.
	atomic.StoreInt32(&sc.dead, 1)
}

func (sc *ShardCtrler) killed() bool {
	z := atomic.LoadInt32(&sc.dead)
	return z == 1
}

// needed by shardkv tester
func (sc *ShardCtrler) Raft() *raft.Raft {
	return sc.rf
}

// Lab 3A 的流程, 收到 applyMsg, 执行 Op, 看是否需要给 waitCh 发通知
func (sc *ShardCtrler) applier() {
	for !sc.killed() {
		applyMsg := <-sc.applyCh
		sc.mu.Lock()
		DPrintf("[%d] receives applyMsg [%v]", sc.me, applyMsg)
		op := applyMsg.Command.(Op)
		sc.execute(&op)
		currentTerm, isLeader := sc.rf.GetState()
		if isLeader && applyMsg.CommandTerm == currentTerm {
			sc.notifyWaitCh(applyMsg.CommandIndex, &op)
		}
		sc.mu.Unlock()
	}
}

// 根据 Op 类别执行相应的操作
func (sc *ShardCtrler) execute(op *Op) {
	DPrintf("[%d] apply command [%+v] success", sc.me, op)
	if sc.isInvalidRequest(op.ClientId, op.RequestId) {
		return
	} else {
		switch op.OpType {
		case Join:
			sc.processJoin(op)
		case Leave:
			sc.processLeave(op)
		case Move:
			sc.processMove(op)
		case Query:
			sc.processQuery(op)
		}
		sc.UpdateLastRequest(op)
	}
}

// 用到的字段为 Servers (new GID -> servers mappings)
// 要求分片控制器创建包含新副本组的新配置, 并分配分片给新的副本组
func (sc *ShardCtrler) processJoin(op *Op) {
	n := len(sc.configs)
	// 注意拷贝一份原来的 Groups
	newGroups, newShards := sc.copyGroups(), sc.configs[n-1].Shards
	// 将新的 Servers 添加到组中
	for gid, servers := range op.Servers {
		newGroups[gid] = servers
	}
	// 调整切片
	sc.adjustShards(newGroups, &newShards)
	// 生成新配置并保存
	newConfig := Config{
		Num:    sc.configs[n-1].Num + 1,
		Shards: newShards,
		Groups: newGroups,
	}
	DPrintf("[%d] processJoin [%v] success", sc.me, newConfig)
	sc.configs = append(sc.configs, newConfig)
}

// 用到的字段为 GIDs (离开的组)
// 应创建一个不包含这些组号的新配置, 并将这些组管理的分片下发给其它组
func (sc *ShardCtrler) processLeave(op *Op) {
	n := len(sc.configs)
	newGroups, newShards := sc.copyGroups(), sc.configs[n-1].Shards
	// 重置切片所属的组并删除副本组
	for _, gid := range op.GIDs {
		for shardId, belongGid := range newShards {
			if gid == belongGid {
				newShards[shardId] = 0
			}
		}
		delete(newGroups, gid)
	}
	sc.adjustShards(newGroups, &newShards)
	// 生成新配置并保存
	newConfig := Config{
		Num:    sc.configs[n-1].Num + 1,
		Shards: newShards,
		Groups: newGroups,
	}
	DPrintf("[%d] processLeave [%v] success", sc.me, newConfig)
	sc.configs = append(sc.configs, newConfig)
}

// 用到的字段为 Shard, GID, 需将分片 Shard 给组 GID
func (sc *ShardCtrler) processMove(op *Op) {
	n := len(sc.configs)
	Shard, GID := op.Shard, op.GID
	// if _, exist := sc.configs[n-1].Groups[GID]; !exist {
	// 	return
	// }
	newGroups, newShards := sc.copyGroups(), sc.configs[n-1].Shards
	newShards[Shard] = GID
	newConfig := Config{
		Num:    sc.configs[n-1].Num + 1,
		Shards: newShards,
		Groups: newGroups,
	}
	DPrintf("[%d] processMove [%v] success", sc.me, newConfig)
	sc.configs = append(sc.configs, newConfig)
}

// 用到的字段为 Num, 返回配置 (需要判断是否为 -1 或大于已知的最大配置号)
func (sc *ShardCtrler) processQuery(op *Op) {
	num, n := op.Num, len(sc.configs)
	if num == -1 || num >= n {
		op.Configure = sc.configs[n-1]
		return
	}
	op.Configure = sc.configs[num]
}

// 拷贝 Groups, 因为 map 是引用对象, 必须深拷贝
func (sc *ShardCtrler) copyGroups() map[int][]string {
	n := len(sc.configs)
	newGroups := make(map[int][]string, len(sc.configs[n-1].Groups))
	for gid, servers := range sc.configs[n-1].Groups {
		copy_servers := make([]string, len(servers))
		copy(copy_servers, servers)
		newGroups[gid] = copy_servers
	}
	return newGroups
}

// 负载均衡处理函数, 有不少坑, 容易出错
func (sc *ShardCtrler) adjustShards(newGroups map[int][]string, newShards *[NShards]int) {
	// 这个条件很重要, 否则若此时已经没有组了的话, 再进行负载均衡的调整会引入 -1 这个无效组 (初始化为 -1)
	// 0 组并不是真正的组, 不会出现在 newGroups 里
	if len(newGroups) == 0 {
		return
	}
	// 得到 gid 到 shardId 的映射关系, 注意必须先让所有的 gid 占个坑, 否则遍历找拥有分片数最大最小的组的时候会引入 -1 这个无效组
	DPrintf("[%d] newGroups [%v], newShards [%v]", sc.me, newGroups, newShards)
	cnt := map[int][]int{}
	for gid := range newGroups {
		cnt[gid] = make([]int, 0)
	}
	for shardId, gid := range newShards {
		cnt[gid] = append(cnt[gid], shardId)
	}
	DPrintf("[%d] cnt [%v]", sc.me, cnt)
	// 不断移动拥有分片数最多的组的分片给拥有分片数最少的分片
	for {
		maxGid, mx, minGid, mn := sc.findMaxAndMinGid(cnt)
		// DPrintf("[%d] maxGid [%d] contains [%d] shard, minGid [%d] contains [%d] shards", sc.me, maxGid, mx, minGid, mn)
		if maxGid != 0 && mx-mn <= 1 {
			return
		}
		DPrintf("[%d] cnt [%+v], newShards [%+v]", sc.me, cnt, newShards)
		shardId := cnt[maxGid][0]
		cnt[maxGid] = cnt[maxGid][1:]
		cnt[minGid] = append(cnt[minGid], shardId)
		newShards[shardId] = minGid
	}
}

// 若 GID 0 有分片则优先处理这些分片, 否则找拥有分片数最大和最小的组
func (sc *ShardCtrler) findMaxAndMinGid(cnt map[int][]int) (int, int, int, int) {
	maxGid, mx, minGid, mn := -1, math.MinInt, -1, math.MaxInt
	// 为了实现确定性遍历, 不这样会出现不同 ShardCtrler 的分片所属情况不一致的情况
	gids := []int{}
	for gid := range cnt {
		gids = append(gids, gid)
	}
	sort.Ints(gids)
	for _, gid := range gids {
		m := len(cnt[gid])
		if maxGid != 0 && ((gid == 0 && m > 0) || (gid != 0 && m > mx)) {
			maxGid, mx = gid, m
		}
		if gid != 0 && m < mn {
			minGid, mn = gid, m
		}
	}
	return maxGid, mx, minGid, mn
}

// 给 waitCh 发送通知，让其生成响应。必须在发送前检查一下 waitCh 是否关闭
func (sc *ShardCtrler) notifyWaitCh(index int, op *Op) {
	DPrintf("[%d] notifyWaitCh [%d]", sc.me, index)
	if waitCh, ok := sc.waitChMap[index]; ok {
		waitCh <- op
	}
}

// 检查当前命令是否为无效的命令 (可能为过期的 RPC 或已经执行过的命令)
func (sc *ShardCtrler) isInvalidRequest(clientId int64, requestId int64) bool {
	if lastRequestId, ok := sc.LastRequestMap[clientId]; ok {
		if requestId <= lastRequestId {
			return true
		}
	}
	return false
}

// 更新对应客户端对应的最近一次请求的Id, 这样可以避免今后执行过期的 RPC 或已经执行过的命令
func (sc *ShardCtrler) UpdateLastRequest(op *Op) {
	lastRequestId, ok := sc.LastRequestMap[op.ClientId]
	if (ok && lastRequestId < op.RequestId) || !ok {
		sc.LastRequestMap[op.ClientId] = op.RequestId
	}
}

// servers[] contains the ports of the set of
// servers that will cooperate via Raft to
// form the fault-tolerant shardctrler service.
// me is the index of the current server in servers[].
func StartServer(servers []*labrpc.ClientEnd, me int, persister *raft.Persister) *ShardCtrler {
	sc := new(ShardCtrler)
	sc.me = me

	sc.configs = make([]Config, 1)
	sc.configs[0].Groups = map[int][]string{}

	labgob.Register(Op{})
	sc.applyCh = make(chan raft.ApplyMsg)
	sc.rf = raft.Make(servers, me, persister, sc.applyCh)

	// Your code here.
	sc.waitChMap = make(map[int]chan *Op)
	sc.LastRequestMap = make(map[int64]int64)
	go sc.applier()

	return sc
}
