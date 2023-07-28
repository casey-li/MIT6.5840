# :two_hearts: Lab 4、Sharded Key/Value Service

[6.5840 Lab 4: Sharded Key/Value Service 实验介绍](https://pdos.csail.mit.edu/6.824/labs/lab-shard.html)

本实验将构建一个**分片键/值存储系统**, 分片是键/值对的子集; 如所有以 “a” 开头的键可能是一个分片, 所有以 “b” 开头的键可能是另一个分片等等。分片后, 每个副本组仅处理几个分片的 `Put` 和 `Append`, 因为这些组可以并行操作, 系统的性能可以得到极大的提升; 此外, 系统总吞吐量 (单位时间的放入和获取) 与组的数量成正比


### :memo: Shard KV 系统组件

- :one: 一组副本组, 每个副本组 (由少数使用 Raft 来复制组分片的服务器组成) 负责处理分片的一个子集
- :two: 分片控制器 (ShardCtrler), 分片控制器决定哪个副本组为哪个分片提供服务, 即管理配置信息 (Config)。配置会随时间变化, 客户端咨询 ShardCtrler 以查找对应 Key 的副本组, 而副本组则咨询 ShardCtrler 以确定其服务的分片。整个系统仅有一个单一的 ShardCtrler, 使用 Raft 实现容错

### :memo: Shard KV 系统功能

分片存储系统必须能够在多个副本组之间转移分片。原因之一是某些组的负载可能远高于其他组, 需要移动分片以实现负载均衡; 二是某些副本组可能会加入或退出系统 (可能会添加新的副本组以增加容量, 或者现有的副本组可能会脱机以进行修复), 因此必须移动分片以继续满足要求

### :memo: 实验挑战

主要挑战是处理重新配置, 即将分片重新分配给副本组。在单个副本组中, 所有组成员必须就客户端的 `Put/Append/Get` 请求在进行重新配置时达成一致
如 Put 可能与重新配置同时到达, 重新配置导致副本组不再对 Put 的 key 对应的分片负责。所以组中的所有副本必须就 Put 发生在重新配置之前还是之后达成一致. 若在重新配置之前, 则 Put 应生效, 并且该分片的新所有者需要看到生效效果; 若在重新配置之后, Put 将不会生效, 客户必须重新请求该 key 的新所有者

**推荐的方法是让每个副本组使用 Raft 不仅记录请求的ID, 还记录重新配置的ID. 需要确保任何时间最多只有一个副本组为一个分片提供服务**

重新配置还涉及到副本组之间的交互, 如在配置 10 中，组 G1 负责分片 S1, 在配置 11 中, 组 G2 负责分片 S1; 在 10 到 11 的重新配置期间, G1 和 G2 必须使用 RPC 将分片 S1 的内容 (键/值对) 从 G1 移动到 G2

- :one: 客户端和服务器之间的交互只能使用 RPC, 服务器的不同实例不允许共享 Go 变量或文件, 因为它们在逻辑上是物理分离的
- :two: 重新配置仅是将分片分配给副本组而不是 Raft 集群成员变更, 不需要实现 Raft 集群成员变更
- :three: Lab 4 的分片存储系统, 分片控制器和 Lab 3 的 kvraft 必须使用相同的 Raft 实现

### **:mag: 提示** 

Lab 4 共有两个实验：
- :one: Lab 4A 实现分片控制器以管理配置
- :two: Lab 4B 实现完整的分片存储系统

---

## :wink: Lab 4A - The Shard controller

ShardCtrler 管理一系列配置, 每个配置都描述了每个分片由哪个副本组管理以及每个副本组包含哪些服务器的信息; 每当需要更改分片分配信息时, ShardCtrler 都会创建新配置, 当键/值客户端和服务器想要了解当前或过去的配置时，它们会请求 ShardCtrler

### :cherry_blossom: 目标

实现 `Join, Leave, Move, Query` RPC 接口以允许管理员 (和测试) 控制 ShardCtrler 添加新的副本组, 消除副本组以及在副本组之间移动分片

- :one: `Join RPC` 用于添加新的副本组, 它的参数是一组唯一的非零副本组标识符 `(GID)` 到服务器名称列表的映射
- :two: `Leave RPC` 用于移除现有的副本组, 它的参数是先前加入的组的 GID 列表 `(GIDs)`
- :three: `Move RPC` 用于将某个分片移动给某个组, 它的参数是分片号 `(Shard)` 和 `GID`
- :four: `Query RPC` 用于查询配置信息, 它的参数是配置号 `(Num)`, 若 `Num == -1` 或大于已知的最大配置号, ShardCtrler 应回复最新配置

除 `Query` 外, ShardCtrler 都应该创建一个新配置以响应请求; 特别的, 新配置应在整个组中尽可能均匀地划分分片, 并应移动尽可能少的分片来实现该目标 (针对 `Join` 添加新组后重新分配分片以及 `Leave` 移除副本组后重新分配分片这两种情况)

第一个配置的编号应为0, 它不包含任何组并且所有分片应分配给 GID 0 (无效的 GID), 之后的配置编号应不断增加; 通常分片数量应明显多于副本组数 (即每个副本组应服务多个分片), 以便可以以相当细的粒度转移负载

### :mag: 提示

- :one: 从一个简化的 kvraft 服务器开始
- :two: ShardCtrler 应能检测出重复的 RPC, 虽然 Lab 4A 的测试不会对此进行检验, 但 Lab 4B 会在不可靠的网络上使用 ShardCtrler, 若 ShardCtrler 不能过滤掉重复的 RPC 就无法通过 Lab 4B 的测试
- :three: 状态机中执行分片负载均衡的代码必须是确定性的, 在 Go 中, `map` 的迭代顺序是不确定的!! 此外, `map` 是引用对象, 若想基于前一个配置创建一个新的配置, 则需要执行深拷贝
- :four: Go 的竞赛检测器 (`go test -race`) 可以帮助您发现错误

需要在 `shardctrler/server.go` 和 `shardctrler/client.go` 中实现分片控制器, 在`shardctrler/common.go` 中实现 `Join, Leave, Move, Query` RPC 接口

### :dart: 思路

Lab 4A 一上来各种概念比较多, 不容易理清, 自己画了一个示意图方便自己理解

![Lab 4A示意图](https://github.com/casey-li/MIT6.5840/blob/main/Lab4/Lab4A/result/pic/4A%E8%AF%B4%E6%98%8E%E5%9B%BE.png?raw=true)

GID 表示了一个副本组的ID, 由多个 KVServer (底层用 Raft, 更准确的来说应该叫 ShardKVServer) 组成; 每个副本组都可以看作为 Lab 3 的 KVServer (对外表现为单一的副本, 只不过 Lab 3 实现的是一个管理所有分片的单个副本组)

Lab 4 中的每个 GID 仅处理一个或多个 Shard, 等价于把 Lab 3 中 KVServer 维护的 KVDB 进行了拆分, 每组 ShardKVServers 仅管理部分分片 ,即部分keys

注意只有管理者可以通过发起 `Join, Leave, Move, Query` 请求来修改配置, 客户端不应该能修改分片键值服务系统的配置

ShardCtrler 主要维护一系列配置信息, 当管理员请求变更配置信息时 ShardCtrler 根据请求生成新的 Config 并保存; 生成新配置时 (调用 `Join, Leave`) 需要考虑负载均衡的问题 (要求不同副本组所负责的分片数目之差不能大于1)

- **负载均衡**

![负载均衡示意图](https://github.com/casey-li/MIT6.5840/blob/main/Lab4/Lab4A/result/pic/4A%E8%B4%9F%E8%BD%BD%E5%9D%87%E8%A1%A1.png?raw=true)

实现负载均衡只需要不断遍历每个组拥有的分片, 确定拥有分片数最大和最少的组 (`maxGroup, minGroup`) 并将 `maxGroup` 中的一个分片给 `minGroup` 直到满足条件即可

注意若此时 GID 0 有分片的话, 不管其它组拥有多少分片一定先移动 GID 0 的分片 (因为 GID 0 为一个无效组, 仅起标识作用)

- **总体流程图**

![总体流程图](https://github.com/casey-li/MIT6.5840/blob/main/Lab4/Lab4A/result/pic/4A%E8%AF%B7%E6%B1%82%E6%B5%81%E7%A8%8B.png?raw=true)

管理者请求 ShardCtrler 变更配置的流程跟 Lab 3A 一模一样, 区别仅在于执行 `Op` 的流程不一样, 重点在于 `Join` 和 `Leave` 后的负载均衡的实现

### :pizza: 数据结构

#### :one: Config 和 request/reply RPC

都在 `shardctrler/common.go` 中给了, `Config` 和所有 `reply` 的结构体都无需修改; 因为需要实现重复检测, 所以所有请求参数中都类似 Lab3 增加了 `RequestId 和 ClientId` 字段

#### :two: Clerk, ShardCtrler 和 Op

和 Lab 3 很像很像, 区别仅在于 Lab 3 的 KVServer 维护的是键值数据库而 ShardCtrler 维护的是一系列配置信息; Op 也类似于 Lab 3, 将所有 request RPC 的参数都塞进去就可以了, 为了获取 Query 查询的结果, 还加入了一个配置字段

```go
// 跟 Lab 3 一样
type Clerk struct {
    servers []*labrpc.ClientEnd
    // Your data here.
    ClientId  int64
    RequestId int64
    LeaderId  int
}

// 同 KVServer, 只不过状态机不再是键值数据库而是 configs
type ShardCtrler struct {
    mu      sync.Mutex
    me      int
    rf      *raft.Raft
    applyCh chan raft.ApplyMsg
    dead           int32
    configs        []Config         // indexed by config num
    waitChMap      map[int]chan *Op // 通知 chan, key 为日志的下标，值为通道
    LastRequestMap map[int64]int64  // 保存每个客户端对应的最近一次请求的 Id
}

// 同 KVServer, 只不过将字段从 K/V 换成了执行 Join/Leave/Move/Query Args 所需的参数
type Op struct {
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
```

### :beers: 实现

#### :cherries: Join / Leave / Move / Query RPC

流程跟 Lab 3 几乎一样, 不过客户端这里给了源文件中给了代码骨架, 根据它的写就可以了。只不过这里的客户端指的是管理员, 而服务端指的是依靠 Raft 实现容错的 ShardCtrler (实际上是 ShardCtrlers)

所有 RPC 都一个模子, 因此仅列举 `Join RPC` 的具体过程

- :one: 客户端

其它函数跟它的区别仅在于参数的构造

```go
func (ck *Clerk) Join(servers map[int][]string) {
    args := &JoinArgs{
        Servers:   servers,
        RequestId: ck.RequestId,
        ClientId:  ck.ClientId,
    }
    for {
        for _, srv := range ck.servers {
            var reply JoinReply
            ok := srv.Call("ShardCtrler.Join", args, &reply)
            if ok && !reply.WrongLeader {
                ck.RequestId += 1
                return
            }
        }
        time.Sleep(100 * time.Millisecond)
    }
}
```

- :two: 服务器

其它函数跟它的区别仅在于 Op 的构造; 此外, 只有 Query 需要返回配置结果

```go
func (sc *ShardCtrler) Join(args *JoinArgs, reply *JoinReply) {
    sc.mu.Lock()
    if sc.isInvalidRequest(args.ClientId, args.RequestId) {
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
    waitChan, exist := sc.waitChMap[index]
    if !exist {
        sc.waitChMap[index] = make(chan *Op, 1)
        waitChan = sc.waitChMap[index]
    }
    sc.mu.Unlock()
    select {
    case res := <-waitChan:
        reply.Err = OK
        currentTerm, stillLeader := sc.rf.GetState()
        if !stillLeader || currentTerm != term {
            reply.WrongLeader = true
        }
    case <-time.After(ExecuteTimeout):
        reply.WrongLeader = true
    }
    sc.mu.Lock()
    delete(sc.waitChMap, index)
    sc.mu.Unlock()
}
```

#### :cherries: 其它函数

总体流程同 Lab 3A, 所以很多函数可以直接拿过来用, 包括 `notifyWaitCh(), isInvalidRequest(), UpdateLastRequest()`

只需修改具体的执行操作即可, 其中 `Query / Move` 比较简单, 主要实现 `Join / Leave` 后的负载均衡处理

```go
// Lab 3A 的流程, 收到 applyMsg, 执行 Op, 看是否需要给 waitCh 发通知
func (sc *ShardCtrler) applier() {
    for !sc.killed() {
        applyMsg := <-sc.applyCh
        sc.mu.Lock()
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
    // 调整切片
    sc.adjustShards(newGroups, &newShards)
    // 生成新配置并保存
    newConfig := Config{
        Num:    sc.configs[n-1].Num + 1,
        Shards: newShards,
        Groups: newGroups,
    }
    sc.configs = append(sc.configs, newConfig)
}

// 用到的字段为 Shard, GID, 需将分片 Shard 给组 GID
func (sc *ShardCtrler) processMove(op *Op) {
    n := len(sc.configs)
    Shard, GID := op.Shard, op.GID
    newGroups, newShards := sc.copyGroups(), sc.configs[n-1].Shards
    // 更改分片所属的组
    newShards[Shard] = GID
    // 生成新配置并保存
    newConfig := Config{
        Num:    sc.configs[n-1].Num + 1,
        Shards: newShards,
        Groups: newGroups,
    }
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
    // 得到 gid 到 shardId 的映射关系
    // 注意必须先让所有的 gid 占个坑, 否则遍历找拥有分片数最大最小的组的时候会引入 -1 这个无效组
    cnt := map[int][]int{}
    for gid := range newGroups {
        cnt[gid] = make([]int, 0)
    }
    for shardId, gid := range newShards {
        cnt[gid] = append(cnt[gid], shardId)
    }
    // 不断移动拥有分片数最多的组的分片给拥有分片数最少的分片
    for {
        maxGid, mx, minGid, mn := sc.findMaxAndMinGid(cnt)
        if maxGid != 0 && mx-mn <= 1 {
            return
        }
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
```

### :sob: 遇到的一些问题

#### :one: 报错无效的组 (case1)

> Test: Basic leave/join ...
--- FAIL: TestBasic (0.31s)
test_test.go:31: shard 0 -> invalid group -1

查看日志后发现问题在于统计每个组拥有的分片的映射关系 (`map[int][]int`)的逻辑那里 (`adjustShards()` 函数); 原本实现为遍历当前每个分片所属的组, 然后将分片加入到那个组中再找拥有分片数最大和最小的组

```go
cnt := map[int][]int{}
for shardId, gid := range newShards {
    cnt[gid] = append(cnt[gid], shardId)
}
```

但是对于新加入的组 Gnew 来说, 没有任何分片属于它, 因此哈希表中不会存在 key 为 Gnew 的项; 第一次调用 Join 时, cnt 中就只有 {0: {0 ~ 9}}, 然后找找拥有分片数最大和最小的组时, 最小的组就会是 -1 (因为初始化为了 -1, 又没有其它组)

**:thought_balloon: 解决方案**

在初始化了 cnt 后, 让所有的 gid 先占个坑就可以了, 这样新加入的组拥有的分片才为空

```go
cnt := map[int][]int{}
for gid := range newGroups {
    cnt[gid] = make([]int, 0)
}
for shardId, gid := range newShards {
    cnt[gid] = append(cnt[gid], shardId)
}
```

#### :two: 报错无效的组 (case2)

> Test: Concurrent leave/join ...
--- FAIL: TestBasic (1.55s)
test_test.go:31: shard 9 -> invalid group -1

查看日志后, 发现问题在于 Lab 4A 的测试案例都是成组成组测的; 一组里面包含多个测试函数, 都是前面的测试函数跑完了以后, 基于之前的结果测试下一个. 因此之前的实现中若缺少特殊条件的判断的话, 就可能会引入 -1 这个无效组

产生错误的原因: Join 和 Leave 在添加或移除拷贝组后会进行分片的负载均衡处理, 自己是比较拥有分片数最大和最小的拷贝组来决定是否迁移分片 (特殊情况是若 0 组有分片则必须将 0 组的分片分给拥有分片数最少的拷贝组, 并且 0 组不能是分片数最少的组; 因为 0 仅仅是一个标志位, 配置中并没有 0 组, `sc.configs[0].Groups` 被初始化为了空)

所以若先 Join 了两组再 Leave 这两组的话, 所有分片会都先还给 0 组 (此时一切正常); 但是随后处理负载均衡时就出问题了 (在 `findMaxAndMinGid()` 中拥有最大和最小分片数的组都被初始化为了 -1 后再遍历, 进行更新), 此时只剩下 0 这一组了, 因此拥有分片数最小的组仍为 -1, 然后所有分片都会被分给 -1, 产生了无效组 !!! 随后测试案例调用 `check()` 检查的时候就报错了

**:thought_balloon: 解决方案**

在调用 adjustShards() 来调整分片时应该新增一个判断条件, 若当前没有有效组的话, 直接返回即可 (注意配置中并没有 0 组, 它仅仅只是作为一个标志位而已, 所以判断 `len(newGroups) == 0` 即可)

#### :three: 分片结果不一致

> Test: Check Same config on servers ...
--- FAIL: TestMulti (0.70s)
    test_test.go:62: Shards wrong

这个 bug 找了很久 :sob:, 最开始以为是代码逻辑错了, 将分片分错了, 但是后面检查的时候发现逻辑并没有问题; 出现错误的原因在于自己忽略了 map 的特性 (它是随机访问保存的键值对的), 不同的服务器在进行负载均衡调整的时候就会产生分片不一致的情况.

最最简单的例子为初始化后, 所有分片尚未分配 (都属于 0 组, G0 管理的分片为 0 ~ 9 ), 此时 Join 了两个组 G1 和 G2, 那么就需要进行负载均衡的调整 (遍历每个组拥有的分片, 找拥有分片数最大和最少的组); 服务器 S1 的访问顺序为 {G1, G2}, 那么 S1 就会把 G0 的第一个分片 0 给 G1 (拥有最多和最少分片数的组为 G0 和 G1); 但是服务器 S2 的访问顺序可能为 {G2, G1}, 那么 S2 就会把 G0 的第一个分片 0 给 G2. 分片结果产生了不一致现象 !!! 

**:thought_balloon: 解决方案**

定义 cnt 为记录了每个组拥有的分片的映射关系 (map[int][]int), 那么在确定拥有分片数最大和最少的组时必须保证所有服务器访问 cnt 的顺序一致, 因此可以先读取所有的组Id, 然后按照组号从小到大或从大到小的顺序进行遍历 

### :rainbow: 结果

![4A 结果](https://github.com/casey-li/MIT6.5840/blob/main/Lab4/Lab4A/result/pic/Lab4A%E7%BB%93%E6%9E%9C.png?raw=true)

通过了 500 次的压力测试，结果保存在 `Lab4A/result/test_4A_500times.txt` 中

---

## :wink: Lab 4B - Sharded Key/Value Server

构建一个分片容错键/值存储系统; 一个副本组由多个 shardkv server 保证容错, 每个副本组相当于仅处理部分 keys 的 Lab 3, 都支持 `Get, Put, Append` 操作 (仅处理维护自己管理的分片中包含的 keys)

客户端使用 `key2shard()` 来查找 key 属于哪个分片, 多个副本组协作以为完整的分片集提供服务

### :cherry_blossom: 目标

构建一个分片键/值存储系统, 修改相关代码以支持分片分配, 配置更改和分片迁移功能, 满足

- :one: 实现多个副本组的协作, 每个副本组仅处理其所负责的分片中的键, 并支持 `Get, Put, Append` 操作
- :two: ShardCtrler 服务单例负责分片的分配和配置信息的维护以确保分片的一致性
- :three: 存储系统需要满足线性一致性要求
- :four: 仅当副本组中的大多数 shardkv server 处于活动状态且可以相互通信, 并且可以与大多数 ShardCtrler server 通信时, 该分片才能正常工作。即使某些副本组中的少数服务器已死亡, 暂时不可用或速度缓慢, 该副本组也必须正常运行 (处理请求并能够根据需要重新配置)
- :five: 实现动态配置更改, 能够检测配置变化并启动分片迁移过程
- :six: 在配置更改期间实施分片迁移, 确保所有服务器在同一点执行迁移, 保证请求的一致性

### :mag: 提示

- :one: 在 `server.go` 中添加代码以定期轮询 ShardCtrler 了解新配置 (大约 100ms 一次; 可以更加频繁的轮询但是不经常轮询可能会出 bug)
- :two: 当服务器被请求到错误分片时, 返回 `ErrWrongGroup` 错误; 确保 `Get, Put, Append` 处理程序在面临并发的重新配置要求时能做出正确决定; 若客户端收到 `ErrWrongGroup`, 是否更改 `RequestId`? 若服务器执行请求时返回 `ErrWrongGroup`, 是否更新 `LastRequestMap`?
- :three: 若测试失败, 可以检查 gob 错误 (如 `"gob: type not registered for interface ..."`); 虽然 Go 并不认为 gob 错误是致命的, 但它们对测试来说是致命的
- :four: 必须按顺序依次执行重新配置并且需要对重新配置的请求进行重复检测
- :five: 当服务器转移分片到到新的副本组后, 它可以继续存储它不再负责的分片信息 (在实际系统中并不允许), 这可能有助于简化您的服务器实现
- :six: 当 G1 在配置更改期间需要用 G2 的分片数据时, G2 在处理日志条目期间的哪个时间点将分片发送给 G1 重要吗？
- :seven: 可以在 RPC 请求或回复中发送整个 map, 这可以简化分片传输的代码; 注意在发送 `map` 时先拷贝一份以避免 data race (因为 map 为引用类型)
- :eight: 注意在配置更改期间, 可能会出现两个分片组互相传送分片的情况 (可能产生死锁)
- :nine: 服务器不应调用 ShardCtrler 的 `Join()`, 测试案例才会调用

服务器需要相互发送 RPC, 以便在配置更改期间传输分片。Config结构包含服务器名，一个 Server 需要一个labrpc.ClientEnd, 以便发送RPC; 使用 make_end() 函数传给 StartServer() 函数将服务器名转换为 ClientEnd

shardkv server 仅是单个副本组的成员, 给定副本组中的服务器集永远不会更改, 即不需要实现 Raft 组成员变更

### :dart: 思路

每个服务器都维护所有分片的状态, 分片的状态根据配置的变化被划分为了以下四种

```go
type ShardState string

const (
    Serving   = "Serving"   // 当前分片正常服务中
    Pulling   = "Pulling"   // 当前分片正在从其它复制组中拉取信息
    BePulling = "BePulling" // 当前分片正在复制给其它复制组
    GCing     = "GCing"     // 当前分片正在等待清除（监视器检测到后需要从拥有这个分片的复制组中删除分片）
)
```

当监听配置时知道配置信息发生了变更后, 修改分片状态即可。具体的分片迁移工作则交由后台监听的协程处理, 本实验中后台监听的协程如下

- :one: `kv.applier()`, 监听 Raft 提交的 applyMsg 并根据 applyMsg 的类别执行不同的操作
- :two: `kv.monitorRequestConfig()`, 定期监听配置信息是否发生改变 (定期向 ShardCtrler 询问最新配置信息)
- :three: `kv.monitorInsert()`, 定期检查是否需要向其它副本组拉取分片, 即检查是否有处于 Pulling 状态的分片
- :four: `kv.monitorGC()`, 定期检查是否有需要等待清理的分片, 用于通知其他副本组自己已经收到了想要的分片, 让其他副本组完成清理工作, 即检查是否有处于 GCing 状态的分片
- :five: `kv.monitorBePulling()`, 定期检查是否有需要被其他副本组拉取的分片, 即检查状态是否有处于 BePulling 状态的分片

在有关分片迁移的函数中, 最重要的监听函数为 `kv.monitorInsert()`, 虽说 `kv.monitorBePulling()` 跟它是相对的, 但是 `kv.monitorBePulling()` 仅负责检查是否被拉取。 若被拉取了的话改变自身状态 (碰到了一个 bug 后新增的后台监听程序，记录在后序部分了), 该函数不会负责发送分片!!!

总体有两套逻辑，其中之一是完成客户端发送的 Put, Get, Append 操作请求 (类似 Lab 3 的 KVRaft, 只不过现在是在分片上进行); 另一套逻辑则是分片迁移

分片迁移过程中设计到了分片的请求, 删除, 新增了两个 RPC 调用

- :one: `GetShards()`, 给调用者返回它们需要的并且当前副本组中分片状态为 BePulling 的fenpian
- :two: `DeleteShards()`, 调用者收到了需要的分片, 调用该 RPC 通知当前副本组可以删除那些分片并更新状态了

#### 分片迁移流程

按照时间线进行划分的话，总体流程如下

(1) `monitorRequestConfig()` 发现了新配置，提交到底层的 Raft 组, 等待达成共识后处理命令, 更新配置号并修改分片状态 (只要有一个分片的状态由 Serving 改成了 Pulling, 那么这个分片必定会在某个副本组中的状态由 Serving 转变为 BePulling)
(2) `monitorInsert()` 在例行检查的过程中发现了当前副本组掌管的分片中有分片的状态转变为了 Pulling, 会跟上一个配置进行对比, 确定在上一个配置中, 该分片由哪个副本组管理 (即应该向哪个副本组发起拉取分片的请求)。确定好副本组后, 发送 `GetShards RPC` 请求
(3) 其他副本组对当前 RPC 的合法性进行检验, 若检验成功则给予其需要的分片结果, 响应 RPC
(4) 当前副本组获取到了需要的分片后, 生成 InsertShard 命令, 提交到底层 Raft 组, 等待达成共识
(5) 达成共识后, 当前副本组的所有服务器会从 `applier()` 中收到命令, 更新分片数据, 并将更新完成的分片的状态改为 GCing
(6) `monitorGC()` 在例行检查的过程中发现了当前副本组掌管的分片中有分片的状态转变为了 GCing, 给之前持有这些分片的副本组发送 `DeleteShard RPC`
(7) 其他副本组执行 DeleteShard, 向底层的 Raft组 提交 DeleteShards 命令, 等待达成共识后处理命令, 将这些分片的状态由 BePulling 改回 Serving。生成响应 RPC
(8) 当前副本组收到正确的响应 RPC (其他副本组完成了清除操作) 后,生成 AdjustShardState 命令并提交到底层 Raft 组, 等待达成共识后处理命令, 将分片状态由 GCing 改回 Serving 

### :pizza: 数据结构

- :one: 各种命令

很多是已经给了的, 为了方便起见将 GetPutAppend 合并到了一起 (GetPutAppendArgs) 并使用了通用的回复 (CommonReply); 

因为有很多命令, 但是每个命令的流程都是一样的 (提交命令 (`StartCommand`), 等待达成共识, 从 `applier()` 中接收命令, 根据命令类别执行对应命令), 所以抽象出了一个共工的命令 Command, 其 data 字段保存对应的参数

```go
const (
    OK             = "OK"
    ErrNoKey       = "ErrNoKey"
    ErrWrongGroup  = "ErrWrongGroup"
    ErrWrongLeader = "ErrWrongLeader"
    OtherErr       = "OtherErr"
)

type Err string

type PutAppendArgs struct {
    Key   string
    Value string
    Op    string // "Put" or "Append"
    RequestId int64
    ClientId  int64
    Gid       int //请求的复制组 Id
}

type PutAppendReply struct {
    Err Err
}

type GetArgs struct {
    Key string
    RequestId int64
    ClientId  int64
    Gid       int //请求的复制组 Id
}

type GetReply struct {
    Err   Err
    Value string
}

type PullShardArgs struct {
    Gid       int   // 复制组ID
    ShardIds  []int // 拉取的分片号
    ConfigNum int   // 配置号
}

type PullShardReply struct {
    Err            Err
    ConfigNum      int
    Shards         map[int]Shard   // 每个分片号对应的完整分片
    LastRequestMap map[int64]int64 // 每个分片对应的最近一次的请求Id
}

type RemoveShardArgs struct {
    ShardIds  []int // 需要删除的分片号
    ConfigNum int
}

type RemoveShardReply struct {
    Err Err
}

type AdjustShardArgs struct {
    ShardIds  []int // 需要删除的分片号
    ConfigNum int
}

type CheckArgs struct {
    ShardIds  []int // 检查的分片号
    ConfigNum int
}

type CheckReply struct {
    Err Err
}

type Command struct {
    CommandType string
    Data        interface{}
}

const (
    Get              = "Get"
    Put              = "Put"
    Append           = "Append"
    AddConfig        = "AddConfig"
    InsertShard      = "InsertShard"
    DeleteShard      = "DeleteShard"
    AdjustShardState = "AdjustShardState"
)

type CommonReply struct {
    Err   Err
    Value string
}

type GetPutAppendArgs struct {
    Key       string
    Value     string
    OpType    string // "Put" or "Append" or "Get"
    RequestId int64  // 客户端请求Id
    ClientId  int64  // 访问的客户端Id
    Gid       int    // 请求的复制组Id
}
```

- :two: ShardKV 以及 分片

Shard 底层维护的数据为键值对, 实现对 get, put, append 的封装

```go
type ShardKV struct {
    mu           sync.Mutex
    me           int
    rf           *raft.Raft
    applyCh      chan raft.ApplyMsg
    make_end     func(string) *labrpc.ClientEnd
    gid          int
    ctrlers      []*labrpc.ClientEnd
    maxraftstate int 

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

type ShardState string

const (
    Serving   = "Serving"   // 当前分片正常服务中
    Pulling   = "Pulling"   // 当前分片正在从其它复制组中拉取信息
    BePulling = "BePulling" // 当前分片正在复制给其它复制组
    GCing     = "GCing"     // 当前分片正在等待清除（监视器检测到后需要从拥有这个分片的复制组中删除分片）
)

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

func MakeShard(state ShardState) *Shard {
    return &Shard{
        ShardKVDB: make(map[string]string),
        State:     state,
    }
}
```


### :beers: 实现

#### :cherries: 客户端 Get, Put, Append 的请求

跟之前的很类似, 给出 `Get()` 的实现

```go
func (ck *Clerk) Get(key string) string {
    args := GetPutAppendArgs{
        Key:       key,
        OpType:    Get,
        RequestId: ck.RequestId,
        ClientId:  ck.ClientId,
    }
    shard := key2shard(key)
    for {
        gid := ck.config.Shards[shard]
        if servers, ok := ck.config.Groups[gid]; ok {
            for si := 0; si < len(servers); si++ {
                srv := ck.make_end(servers[si])
                var reply GetReply
                ok := srv.Call("ShardKV.Get", &args, &reply)
                if ok && (reply.Err == OK || reply.Err == ErrNoKey) {
                    ck.RequestId++
                    return reply.Value
                }
                if ok && (reply.Err == ErrWrongGroup) {
                    break
                }
            }
        }
        time.Sleep(100 * time.Millisecond)
        ck.config = ck.sm.Query(-1)
    }
}
```

#### :cherries: 服务端 Get, Put, Append 的响应

跟之前 3A 的很类似, 不过凝练出了它们共有的部分 (提交命令等待底层 Raft 达成共识并执行命令, 若未能在规定时间内完成则返回), 因为分片迁移那部分的逻辑也要用到这部分逻辑

此外, 新增了检查当前副本组是否负责管理所在分片的逻辑, 并且提交的命令是经过封装的命令, 即 Command

```go
// 检查当前组是否仍负责管理该分片
// 其它流程同 3A
func (kv *ShardKV) Get(args *GetPutAppendArgs, reply *GetReply) {
    kv.mu.Lock()
    shardId := key2shard(args.Key)
    if !kv.checkShardAndState(shardId) {
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
}

// 检查当前组是否仍负责管理该分片
// Put 或 Append 还需要先检查是否为过期的 RPC 或已经执行过的命令，避免重复执行
func (kv *ShardKV) PutAppend(args *GetPutAppendArgs, reply *PutAppendReply) {
    kv.mu.Lock()
    // 检查是否由自己负责
    shardId := key2shard(args.Key)
    if !kv.checkShardAndState(shardId) {
        reply.Err = ErrWrongGroup
        kv.mu.Unlock()
        return
    }
    if kv.isInvalidRequest(args.ClientId, args.RequestId) {
        reply.Err = OK
        kv.mu.Unlock()
        return
    }
    kv.mu.Unlock()
    command := Command{
        CommandType: args.OpType,
        Data:        *args,
    }
    response := &CommonReply{Err: OK}
    kv.StartCommand(command, response)
    reply.Err = response.Err
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
    waitChan, exist := kv.waitChMap[index]
    if !exist {
        kv.waitChMap[index] = make(chan *CommonReply, 1)
        waitChan = kv.waitChMap[index]
    }
    kv.mu.Unlock()

    select {
    case res := <-waitChan:
        response.Value, response.Err = res.Value, res.Err
        currentTerm, stillLeader := kv.rf.GetState()
        if !stillLeader || currentTerm != term {
            response.Err = ErrWrongLeader
        }
    case <-time.After(ExecuteTimeout):
        response.Err = ErrWrongLeader
    }

    kv.mu.Lock()
    delete(kv.waitChMap, index)
    kv.mu.Unlock()
}
```
#### :cherries: 分片迁移逻辑

这部分就是最难的, bug 都出自这部分 :sob:, 分为两块儿描述分块迁移的逻辑。总体流程已经在思路部分给出了

- :one: 后台监听协程 (监听配置以及分片的状态信息)

```go
// Leader 节点需定期向 ShardCtrler 询问最新配置信息
// 若当前正在更改自身配置则放弃本次询问
// 若发现获取的配置为新配置则更新自身配置
func (kv *ShardKV) monitorRequestConfig() {
    for !kv.killed() {
        // 不能直接退出, 因为不能保证 Leader 不做更改
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
        kv.mu.Unlock()
        if !isProcessShardCommand {
            // 注意这里不能请求最新的
            newConfig := kv.manager.Query(currentConfigNum + 1)
            if newConfig.Num == currentConfigNum+1 {
                reply := &CommonReply{}
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
        // 用于等待其他副本组回复
        wg := &sync.WaitGroup{}
        wg.Add(len(groups))
        for oldGid, shardIds := range groups {
            configNum, servers := kv.currentCongig.Num, kv.lastConfig.Groups[oldGid]
            // 增加配置号以避免更改配置的请求被重复执行
            go func(oldGid int, configNum int, servers []string, shardIds []int) {
                defer wg.Done()
                // 找 Leader
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
                        kv.StartCommand(command, &CommonReply{})
                    }
                }
            }(oldGid, configNum, servers, shardIds)
        }
        kv.mu.Unlock()
        wg.Wait()
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
            _, ok := kv.shards[shardId]
            if ok && kv.shards[shardId].State == BePulling {
                shards[shardId] = kv.shards[shardId].CopyShard()
            }
        }
        reply.Err, reply.Shards, reply.LastRequestMap = OK, shards, kv.copyLastRequestMap()
    } else {
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
        for oldGid, shardIds := range groups {
            configNum, servers := kv.currentCongig.Num, kv.lastConfig.Groups[oldGid]
            // 增加配置号以避免更改配置的请求被重复执行
            go func(oldGid int, configNum int, servers []string, shardIds []int) {
                defer wg.Done()
                for _, server := range servers {
                    args := &RemoveShardArgs{
                        ShardIds:  shardIds,
                        ConfigNum: configNum,
                    }
                    reply := &RemoveShardReply{}
                    srv := kv.make_end(server)
                    ok := srv.Call("ShardKV.DeleteShards", args, reply)
                    if ok && reply.Err == OK {
                        adjargs := AdjustShardArgs {
                            ShardIds:  shardIds,
                            ConfigNum: configNum,
                        }
                        command := Command{
                            CommandType: AdjustShardState,
                            Data:        adjargs,
                        }
                        kv.StartCommand(command, &CommonReply{})
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
    kv.StartCommand(command, response)
    reply.Err = response.Err
}

// 为了解决 bug 而增加的一个监听协程
// 检查当前分组是否有处于 BePulling 状态的分片, 如果有的话询问 Pulling 的副本组, 问他是否拉取成功, 若拉取成功则修改分片状态为 Serving
func (kv *ShardKV) monitorBePulling() {
    for !kv.killed() {
        if _, isLeader := kv.rf.GetState(); !isLeader {
            time.Sleep(100 * time.Millisecond)
            continue
        }
        kv.mu.Lock()
        id, gid := kv.me, kv.gid
        groups := make(map[int][]int)
        // 注意是看当前配置中的分片状态, 因为并不是给别人发送
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
        for oldGid, shardIds := range groups {
            configNum, servers := kv.currentCongig.Num, kv.lastConfig.Groups[oldGid]
            // 增加配置号以避免更改配置的请求被重复执行
            go func(oldGid int, configNum int, servers []string, shardIds []int) {
                defer wg.Done()
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
                    }
                }
            }(oldGid, configNum, servers, shardIds)
        }
        kv.mu.Unlock()
        wg.Wait()
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

```

- :two: 处理函数

本部分描述收到了命令后如何处理以修改分片的状态, 因为若要修改状态底层 Raft 组必须达成一致, 因此命令必定是在 applier 收到 Raft 提交的 ApplyMsg 后才能执行

```go
// 监听 Raft 提交的 applyMsg, 根据 applyMsg 的类别执行不同的操作
// 为命令的话，必执行，执行完后检查是否需要给 waitCh 发通知
// 为快照的话读快照，更新状态
func (kv *ShardKV) applier() {
    for !kv.killed() {
        applyMsg := <-kv.applyCh
        kv.mu.Lock()
        id, gid := kv.me, kv.gid
        if applyMsg.CommandValid {
            if applyMsg.CommandIndex <= kv.lastIncludedIndex {
                kv.mu.Unlock()
                continue
            }
            reply := kv.execute(applyMsg.Command)
            currentTerm, isLeader := kv.rf.GetState()
            if isLeader && applyMsg.CommandTerm == currentTerm {
                kv.notifyWaitCh(applyMsg.CommandIndex, reply)
            }
            if kv.maxraftstate != -1 && kv.persister.RaftStateSize() >= kv.maxraftstate {
                kv.rf.Snapshot(applyMsg.CommandIndex, kv.encodeState())
            }
            kv.lastIncludedIndex = applyMsg.CommandIndex
        } else if applyMsg.SnapshotValid {
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

// 根据 OpType 选择操作执行
// 跟之前的区别在于新增了不少命令类别并且把 GetPutAppend 放到一起处理了
func (kv *ShardKV) execute(cmd interface{}) *CommonReply {
    command := cmd.(Command)
    reply := &CommonReply{
        Err: OK,
    }
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
        kv.processAdjustGCingShard(&args, reply)
    }
    return reply
}

// 先检查此时该 key 是否仍由自己负责
// 执行命令，若为重复命令并且不是 Get 的话，直接返回即可
// 否则根据 OpType 执行命令并更新该客户端最近一次请求的Id
func (kv *ShardKV) processGetPutAppend(op *GetPutAppendArgs, reply *CommonReply) {
    shardId := key2shard(op.Key)
    if !kv.checkShardAndState(shardId) {
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
        kv.UpdateLastRequest(op)
    }
}

// 检查是否是真的最新的配置 (配置号应该比自己的更大一号)
// 检查所有分片的所属情况，若需要从其它副本组拉去分片信息或者给其它副本组发送分片信息的话修改分片状态
func (kv *ShardKV) processAddConfig(newConfig *shardctrler.Config, reply *CommonReply) {
    if newConfig.Num == kv.currentCongig.Num+1 {
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
        }
        kv.lastConfig = kv.currentCongig
        kv.currentCongig = *newConfig
        reply.Err = OK
    } else {
        reply.Err = OtherErr
    }
}

// 先判断配置号是否相等
// 依次更新每个分片中维护的键值数据库
// 更新每个分片对应的客户端最近一次请求的Id, 避免重复执行命令
func (kv *ShardKV) processInsertShard(response *PullShardReply, reply *CommonReply) {
    if response.ConfigNum == kv.currentCongig.Num {
        Shards := response.Shards
        for shardId, shard := range Shards {
            oldShard := kv.shards[shardId]
            if oldShard.State == Pulling {
                for key, value := range shard.ShardKVDB {
                    oldShard.ShardKVDB[key] = value
                }
                oldShard.State = GCing
            }
        }
        LastRequestMap := response.LastRequestMap
        for clientId, requestId := range LastRequestMap {
            kv.LastRequestMap[clientId] = max(requestId, kv.LastRequestMap[clientId])
        }
    } else {
        reply.Err = OtherErr
    }
}

// 先判断配置号是否相等
// 若当前分片确实原本由自己负责并且状态为待拉取状态的话就重置该分片
func (kv *ShardKV) processDeleteShard(args *RemoveShardArgs, reply *CommonReply) {
    if args.ConfigNum == kv.currentCongig.Num {
        for _, shardId := range args.ShardIds {
            _, ok := kv.shards[shardId]
            if ok && kv.shards[shardId].State == BePulling {
                kv.shards[shardId] = MakeShard(Serving)
            }
        }
    } else {
        reply.Err = OtherErr
        if args.ConfigNum < kv.currentCongig.Num {
            reply.Err = OK
        }
    }
}

func (kv *ShardKV) processAdjustGCingShard(args *AdjustShardArgs, reply *CommonReply) {
    if args.ConfigNum == kv.currentCongig.Num {
        for _, shardId := range args.ShardIds {
            if _, ok := kv.shards[shardId]; ok {
                kv.shards[shardId].State = Serving
            }
        }
    } else {
        reply.Err = OtherErr
        if args.ConfigNum < kv.currentCongig.Num {
            reply.Err = OK
        }
    }
}
```

### :sob: 遇到的一些问题

#### :one: 死锁，分片无法迁移，彼此等待 (case1)

检查日志后发现问题在于 `monitorRequestConfig()` 中自己请求新配置时直接请求的最新配置 (`kv.manager.Query(-1)`), 可能会跳过某些配置 (比如当前配置号为2, 最新的为 5), 而某个服务器处于配置 3, 正在等待当前服务器进入配置 3 并拉取分片。 在最新配置中可能当前服务器又要向其他服务器拉取分片, 但是其他服务器的配置号远远落后于当前服务器的配置号, 因此不会回应。服务器之间的配置信息变更产生了死锁现象，彼此都不能对外提供服务，最终超时。

**:thought_balloon: 解决方案**

每次请求的配置应该为当前配置的下一个配置, 这样就不会跳过一些配置
> newConfig := kv.manager.Query(currentConfigNum + 1)

#### :two: 死锁，分片无法迁移，彼此等待 (case2)

检查日志后发现问题在于服务器重启后因为进度回退的不一致会造成死锁现象。即虽然某个配置下分片已经正常完成了迁移, 但是因为快照记录的时间点不同, 不同服务器回退到了不同的状态。比如当前服务器退回到了待拉取分片的状态, 那么不会有其他服务器拉取该服务器的分片, 该服务器的状态就得不到更新, 导致所有服务器的状态都停在那里。最后远远落后于客户端拉取到的配置号

一个例子如下

```
[id, gid]   reach agreement {Num:24 Shards:[100 102 102 102 102 100 102 100 100 100]}

[1, 102]    receive new Config {Num:25 Shards:[100 100 100 100 100 100 100 100 100 100]}

[2, 101]    receive new Config {Num:25 Shards:[100 100 100 100 100 100 100 100 100 100]} 

[1, 102]    receive AddConfig, updates shards state [Serving, BePulling, BePulling, BePulling, BePulling, Serving, BePulling, Serving, Serving, Serving]
[0/2, 102]  reach agreement on new Config

[2, 101]    receive AddConfig, updates shards state [Serving, Serving, Serving, Serving, Serving, Serving, Serving, Serving, Serving, Serving]
[0/1, 101]  reach agreement on new Config

[2, 100]    receive new Config {Num:25 Shards:[100 100 100 100 100 100 100 100 100 100]}

[2, 100]    receive AddConfig, updates shards state [Serving, Pulling, Pulling, Pulling, Pulling, Serving, Pulling, Serving, Serving, Serving]
[0/1, 100]  reach agreement on new Config

[2, 100]    pull shards to 102, shardIds [3 2 1 4 6] and get shards
[2, 100]    commit  InsertShard command
[2, 100]    receive InsertShard command, updates shards state [Serving, GCing, GCing, GCing, GCing, Serving, GCing, Serving, Serving, Serving]
[0/1, 100]  reach agreement on InsertShard command

[2, 101]    receive new Config {Num:26 Shards:[102 102 102 102 102 100 100 100 100 100]}

[2, 101]    receive AddConfig, updates shards state [Serving, Serving, Serving, Serving, Serving, Serving, Serving, Serving, Serving, Serving]
[0/1, 101]  reach agreement on new Config

[2, 100]    send DeleteShard command to 102, shardIds [3 2 1 4 6]

[1, 102]    commit  DeleteShard command
[1, 102]    receive DeleteShard  command, delete shards, state [Serving, Serving, Serving, Serving, Serving, Serving, Serving, Serving, Serving, Serving]
[0/2, 102]  reach agreement on DeleteShard command

[2, 100]    commit AdjustShardState command
[2, 100]    receive AdjustShardState command, updates shards state [Serving, Serving, Serving, Serving, Serving, Serving, Serving, Serving, Serving, Serving]

[2, 100]    receive new Config {Num:26 Shards:[102 102 102 102 102 100 100 100 100 100]}
[2, 100]    receive AddConfig, updates shards state [BePulling, BePulling, BePulling, BePulling, BePulling, Serving, Serving, Serving, Serving, Serving]

[2, 101]    receive new Config {Num:27 Shards:[101 102 102 102 102 101 101 100 100 100]}

[2, 101]    receive AddConfig, updates shards state [Pulling, Serving, Serving, Serving, Serving, Pulling, Pulling, Serving, Serving, Serving]

// 所有机器重启了...

[0, 100]    init config {Num:26 Shards:[102 102 102 102 102 100 100 100 100 100]}
[1, 100]    init config {Num:26 Shards:[102 102 102 102 102 100 100 100 100 100]}
[2, 100]    init config {Num:26 Shards:[102 102 102 102 102 100 100 100 100 100]}
[0, 101]    init config {Num:26 Shards:[102 102 102 102 102 100 100 100 100 100]}
[1, 101]    init config {Num:26 Shards:[102 102 102 102 102 100 100 100 100 100]}
[2, 101]    init config {Num:27 Shards:[101 102 102 102 102 101 101 100 100 100]}
[0, 102]    init config {Num:25 Shards:[100 100 100 100 100 100 100 100 100 100]}
[1, 102]    init config {Num:25 Shards:[100 100 100 100 100 100 100 100 100 100]}
[2, 102]    init config {Num:25 Shards:[100 100 100 100 100 100 100 100 100 100]}

// 三个 leader 的分片情况


{Num:24 Shards:[100 102 102 102 102 100 102 100 100 100]}
{Num:25 Shards:[100 100 100 100 100 100 100 100 100 100]}
{Num:26 Shards:[102 102 102 102 102 100 100 100 100 100]}

[0, 101] Num:26, Shards:[102 102 102 102 102 100 100 100 100 100]
[0, 101] shards [0: Serving, 1: Serving, 2: Serving, 3: Serving, 4: Serving, 5: Serving, 6: Serving, 7: Serving, 8: Serving, 9: Serving]

[0, 102] Num:25, Shards:[100 100 100 100 100 100 100 100 100 100]
[0, 102] shards [0: Serving, 1: BePulling, 2: BePulling, 3: BePulling, 4: BePulling, 5: Serving, 6: BePulling, 7: Serving, 8: Serving, 9: Serving]

[2, 100] Num:26 {Num:26 Shards:[102 102 102 102 102 100 100 100 100 100]}
[2, 100] shards [0: BePulling, 1: BePulling, 2: BePulling, 3: BePulling, 4: BePulling, 5: Serving, 6: Serving, 7: Serving, 8: Serving, 9: Serving]

// 101 重新接收 27 的配置, 更新状态为 [Pulling, Serving, Serving, Serving, Serving, Pulling, Pulling, Serving, Serving, Serving]
// 100 在等待其他复制组请求删除分片, 102 也在等待其他复制组删除分片, 101 请求拉取分片信息 (但因为100 和 102 都在旧的配置中等待更新配置, 不会给予相应)
// 阻塞在这里直到超时
```

**:thought_balloon: 解决方案**

设置了一个新的主动检测 BePulling 的协程，注意应该给当前配置下 shardId 对应的服务器发送 RPC

#### :three: 超时, 还是死锁了 :sob:

检查日志后发现问题在于设置的管理分片迁移的监听函数, 因为只有 Leader 才应该去不断检查并做更改, 所以自己在最开始就先检查当前服务器是否为 Leader, 若不是 Leader 的话直接退出。但是这样有一个问题是可能一开始还没选出 Leader, 所有监听函数就都退出了; 或者之后 Leader 发生了变更, 然后也没有服务器监听了 

**:thought_balloon: 解决方案**

若当前服务器非 Leader 的话, 应该休眠一段时间再次检查而不是直接退出


### :rainbow: 结果

![4B 结果](https://github.com/casey-li/MIT6.5840/blob/main/Lab4/Lab4B/result/pic/Lab4B%E7%BB%93%E6%9E%9C.png?raw=true)

通过了 500 次的压力测试，结果保存在 `Lab4B/result/test_4B_500times.txt` 中

---

## :rainbow: 最终结果

通过了 500 次的压力测试，结果保存在 Lab4/result/test_4_500times.txt 中

# :rose: 参考

:one: [MIT-6.824-lab4A-2022(万字讲解-代码构建)](https://blog.csdn.net/weixin_45938441/article/details/125386091?ops_request_misc=%257B%2522request%255Fid%2522%253A%2522168925631216800180664720%2522%252C%2522scm%2522%253A%252220140713.130102334..%2522%257D&request_id=168925631216800180664720&biz_id=0&utm_medium=distribute.pc_search_result.none-task-blog-2~all~sobaiduend~default-1-125386091-null-null.142^v88^control_2,239^v2^insert_chatgpt&utm_term=MIT%206.824%20Lab4&spm=1018.2226.3001.4187)

:two: [MIT-6.824-lab4B-2022(万字思路讲解-代码构建)](https://blog.csdn.net/weixin_45938441/article/details/125566763?ops_request_misc=%257B%2522request%255Fid%2522%253A%2522168925631216800180664720%2522%252C%2522scm%2522%253A%252220140713.130102334..%2522%257D&request_id=168925631216800180664720&biz_id=0&utm_medium=distribute.pc_search_result.none-task-blog-2~all~sobaiduend~default-2-125566763-null-null.142^v88^control_2,239^v2^insert_chatgpt&utm_term=MIT%206.824%20Lab4&spm=1018.2226.3001.4187)

:three: [MIT6.824-2021 Lab4 : MultiRaft](https://zhuanlan.zhihu.com/p/463146544)

:four: [MIT 6.824 Lab4 AB 完成记录](https://blog.csdn.net/qq_40443651/article/details/118034894?ops_request_misc=%257B%2522request%255Fid%2522%253A%2522168925631216800180664720%2522%252C%2522scm%2522%253A%252220140713.130102334..%2522%257D&request_id=168925631216800180664720&biz_id=0&utm_medium=distribute.pc_search_result.none-task-blog-2~all~sobaiduend~default-4-118034894-null-null.142^v88^control_2,239^v2^insert_chatgpt&utm_term=MIT%206.824%20Lab4&spm=1018.2226.3001.4187)
