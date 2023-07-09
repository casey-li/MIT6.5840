# [MIT6.5840](https://pdos.csail.mit.edu/6.824/index.html)

记录自己对于不同 Lab 的理解，思路以及遇到的一些 bug，方便回顾

所有 Lab 都通过了 500 次的连续测试，但是无法保证不存在 bug :joy:，肯定仍存在一些特殊情况自己没有考虑到（比如做 Lab2C 的时候因为网络问题就发现了不少 Lab 2A 和 Lab2B 中的 bug，比如缺乏对于过期 RPC 回复的判断）

## :two_hearts: [Lab1 MapReduce](https://github.com/casey-li/MIT6.5840/tree/main/Lab1)

[6.5840 Lab 1: MapReduce](https://pdos.csail.mit.edu/6.824/labs/lab-mr.html)

[MapReduce 论文](extension://bfdogplmndidlpjfhoijckpakkdjkkil/pdf/viewer.html?file=http%3A%2F%2Fstatic.googleusercontent.com%2Fmedia%2Fresearch.google.com%2Fzh-CN%2F%2Farchive%2Fmapreduce-osdi04.pdf)

第一个 lab，难度不是很大，要求实现一个简易版的分布式 mapreduce demo, 只要照着论文中的架构图写就可以了

## :two_hearts: [Lab2 Raft](https://github.com/casey-li/MIT6.5840/tree/main/Lab2)

[6.5840 Lab 2: Raft](https://pdos.csail.mit.edu/6.824/labs/lab-raft.html)

[Raft 论文](extension://bfdogplmndidlpjfhoijckpakkdjkkil/pdf/viewer.html?file=https%3A%2F%2Fpdos.csail.mit.edu%2F6.824%2Fpapers%2Fraft-extended.pdf)

要求实现 Raft 共识算法, 相比于 Lab 1, 难度有了极大提升。共包含四个小实验，论文中的 Fig.2 特别特别重要

- :one: Lab2A 要求实现领导选举和心跳函数。满足在非崩溃情况下，选出的领导者继续担任领导者；并在旧领导者瘫痪或旧领导者的数据包丢失时能够选出新领导者进行接管
- :two: Lab2B 要求在 Lab2A 的基础上增加日志复制功能，此时才算完善了 Lab2A 中的 `RequestVote RPC` 和 `AppendEntries RPC`。follower 在原有逻辑上仅给日志至少跟自己一样新的 Candidate 投票，并且follower 在 `AppendEntries RPC` 中需要对比日志情况来往自己的日志中追加新条目，leader 以及 follower 都需要在满足要求后提交新条目
- :three: Lab2C 要求在 Lab2B 的基础上增加持久化处理，让发生崩溃的服务器重启后能够快速在其发生中断的地方恢复服务。需要持久化处理的字段为任期、投票结果和日志，因此仅需在这三个字段修改的地方调用一下持久化函数即可 (若之前的 Lab2A 和 2B 都通过了很多次测试没出错的话，Lab2C 很快就可以完成，但若前面有 bug 的话可能会卡很久)
- :four: Lab2D 在前面的基础上引入了快照，即实现日志压缩的功能来减小服务器的存储压力。本实验因为涉及到了日志的裁减，所以第一点就是需要实现日志的真实下标和逻辑下标之间的转换逻辑。此外，还需要实现 `InstallSnapshot RPC` 并修改心跳函数，让其根据情况给 follower 发送快照或者日志

因为后序实验都是在前面的实验上进行改进，所以每完成一个实验一定一定要进行压力测试，尽可能的修改掉当前引入的 bug，这样做后面的实验会轻松很多。自己就是最开始没注意这个，跑了几次都过了就继续往后做，到了 Lab2C 以后出现了一堆 bug :sob:，然后又重头回去重写了 Lab2A 和 Lab2B

## :two_hearts: Lab3

[6.5840 Lab 3: Fault-tolerant Key/Value Service](https://pdos.csail.mit.edu/6.824/labs/lab-kvraft.html)

TODO

## :two_hearts: Lab4

[6.5840 Lab 4: Sharded Key/Value Service](https://pdos.csail.mit.edu/6.824/labs/lab-shard.html)

TODO