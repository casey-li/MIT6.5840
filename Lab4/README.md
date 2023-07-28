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

![4A 结果]()

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


#### 流程图


### :pizza: 数据结构


### :beers: 实现
#### :cherries: 

#### :cherries: 

### :rainbow: 结果


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

设置了一个新的主动检测 BePulling 的协程，注意应该给当前配置下 shardId 对应的服务器发送 RPC

---

## 最终结果


---

# :rose: 参考

:one: [MIT-6.824-lab4A-2022(万字讲解-代码构建)](https://blog.csdn.net/weixin_45938441/article/details/125386091?ops_request_misc=%257B%2522request%255Fid%2522%253A%2522168925631216800180664720%2522%252C%2522scm%2522%253A%252220140713.130102334..%2522%257D&request_id=168925631216800180664720&biz_id=0&utm_medium=distribute.pc_search_result.none-task-blog-2~all~sobaiduend~default-1-125386091-null-null.142^v88^control_2,239^v2^insert_chatgpt&utm_term=MIT%206.824%20Lab4&spm=1018.2226.3001.4187)

:two: [MIT-6.824-lab4B-2022(万字思路讲解-代码构建)](https://blog.csdn.net/weixin_45938441/article/details/125566763?ops_request_misc=%257B%2522request%255Fid%2522%253A%2522168925631216800180664720%2522%252C%2522scm%2522%253A%252220140713.130102334..%2522%257D&request_id=168925631216800180664720&biz_id=0&utm_medium=distribute.pc_search_result.none-task-blog-2~all~sobaiduend~default-2-125566763-null-null.142^v88^control_2,239^v2^insert_chatgpt&utm_term=MIT%206.824%20Lab4&spm=1018.2226.3001.4187)
