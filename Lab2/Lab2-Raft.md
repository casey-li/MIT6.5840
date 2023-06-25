# Lab 2、Raft

[有关 Raft 工作流程的动画网址](http://thesecretlivesofdata.com/raft/#home)，有助于快速理解 Raft

下面的博客分四部分介绍了 Raft 的实现，讲的很好 ！！！

:cat: [Part 0 - Introduction](https://eli.thegreenplace.net/2020/implementing-raft-part-0-introduction/)
:rabbit: [Part 1 - Elections](https://eli.thegreenplace.net/2020/implementing-raft-part-1-elections/)，讲解了状态之间的转移（follower、leader 和 candidate），RPC请求（RequestVotes、AppendEntries）和响应，注意本部分并未涉及日志的相关内容
:wolf: [Part 2 - Commands and Log Replication](https://eli.thegreenplace.net/2020/implementing-raft-part-2-commands-and-log-replication/)，主要讲解当一个客户给 leader 发送命令后，leader 如何处理并通知 follower 复制日志；以及 follow 收到 leader 的 AE 请求后，如何处理
:snake: [Part 3 - Persistence and Optimizations](https://eli.thegreenplace.net/2020/implementing-raft-part-3-persistence-and-optimizations/)


Lab2 系列为 Raft 分布式一致性协议算法的实现，Raft 将分布式一致性共识分解为若干个子问题
- leader election，领导选举 (Lab 2A)
- log replication，日志复制 (Lab 2B)
- safety，安全性(Lab 2B & 2C)；2C 除了持久化还有错误日志处理

以上为 raft 的核心特性，除此之外，要用于生产环境，还有许多地方可以优化
- log compaction，日志压缩-快照(lab2D)
- Cluster membership changes，集群成员变更

## Lab 2A - leader election

**:cherry_blossom: 目标**：实现 Raft 的 leader election 和 heartbeats (没有日志条目的 `AppendEntries` RPC)。

**:cherry_blossom: 效果**：选出一个单一的领导者，如果没有瘫痪，领导者继续担任领导者，如果旧领导者瘫痪或旧领导者的数据包丢失，则由新领导者接管丢失

**要求中反复提及注意图2**

![Fig 2](https://github.com/SwordHarry/MIT6.824_2021_note/raw/main/lab/img/008i3skNgy1gvajftq7jmj60u00xyk1402.png)

注意实验提示中说明了测试器将心跳限制为了每秒 10 次，并且要求在旧领导失败后的 5s 内必须选出新的领导者，因此**必须使用比论文中 150 - 300 ms 更大的选举超时时间**，但也不能太大

### 各个角色的职责

总共包含三个角色：leader, follower, candidate
- leader 负责周期性地广播发送 AppendEntries RPC 请求
- candidate 负责周期性地广播发送 RequestVote RPC 请求
- follower 仅负责被动地接收 RPC 请求，从不主动发起请求

### 需要实现的 RPC 接口
- AppendEntries RPC
- RequestVote RPC

### 周期性调用
- election
- heart-beats

