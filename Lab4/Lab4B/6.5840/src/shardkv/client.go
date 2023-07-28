package shardkv

//
// client code to talk to a sharded key/value service.
//
// the client first talks to the shardctrler to find out
// the assignment of shards (keys) to groups, and then
// talks to the group that holds the key's shard.
//

import (
	"crypto/rand"
	"math/big"
	"time"

	"6.5840/labrpc"
	"6.5840/shardctrler"
)

// which shard is a key in?
// please use this function,
// and please do not change it.
func key2shard(key string) int {
	shard := 0
	if len(key) > 0 {
		shard = int(key[0])
	}
	shard %= shardctrler.NShards
	return shard
}

func nrand() int64 {
	max := big.NewInt(int64(1) << 62)
	bigx, _ := rand.Int(rand.Reader, max)
	x := bigx.Int64()
	return x
}

type Clerk struct {
	sm       *shardctrler.Clerk
	config   shardctrler.Config
	make_end func(string) *labrpc.ClientEnd
	// You will have to modify this struct.
	ClientId  int64
	RequestId int64
}

// the tester calls MakeClerk.
//
// ctrlers[] is needed to call shardctrler.MakeClerk().
//
// make_end(servername) turns a server name from a
// Config.Groups[gid][i] into a labrpc.ClientEnd on which you can
// send RPCs.
func MakeClerk(ctrlers []*labrpc.ClientEnd, make_end func(string) *labrpc.ClientEnd) *Clerk {
	ck := new(Clerk)
	ck.sm = shardctrler.MakeClerk(ctrlers)
	ck.make_end = make_end
	// You'll have to add code here.
	ck.ClientId = nrand()
	ck.RequestId = 0
	ck.config = ck.sm.Query(-1) // 请求最新配置信息
	DPrintf("client [%d] init config = %+v", ck.ClientId, ck.config)
	return ck
}

// fetch the current value for a key.
// returns "" if the key does not exist.
// keeps trying forever in the face of all other errors.
// You will have to modify this function.
func (ck *Clerk) Get(key string) string {
	// args := GetArgs{
	// 	Key:       key,
	// 	RequestId: ck.RequestId,
	// 	ClientId:  ck.ClientId,
	// }
	// args.Key = key
	args := GetPutAppendArgs{
		Key:       key,
		OpType:    Get,
		RequestId: ck.RequestId,
		ClientId:  ck.ClientId,
	}

	shard := key2shard(key)
	DPrintf("client [%d] send get args [%+v], shardId [%d]", ck.ClientId, args, shard)

	for {
		gid := ck.config.Shards[shard]
		if servers, ok := ck.config.Groups[gid]; ok {
			// try each server for the shard.
			for si := 0; si < len(servers); si++ {
				srv := ck.make_end(servers[si])
				var reply GetReply
				ok := srv.Call("ShardKV.Get", &args, &reply)
				if ok && (reply.Err == OK || reply.Err == ErrNoKey) {
					DPrintf("client [%d] receive get resply %s", ck.ClientId, reply.Err)
					ck.RequestId++
					return reply.Value
				}
				if ok && (reply.Err == ErrWrongGroup) {
					DPrintf("client [%d] send Get request to worong group %d", ck.ClientId, gid)
					break
				}
				// ... not ok, or ErrWrongLeader
			}
		}
		time.Sleep(100 * time.Millisecond)
		// ask controler for the latest configuration.
		ck.config = ck.sm.Query(-1)
		DPrintf("client [%d] update config [%+v]", ck.ClientId, ck.config)
	}

	// return ""
}

// shared by Put and Append.
// You will have to modify this function.
func (ck *Clerk) PutAppend(key string, value string, op string) {
	// args := PutAppendArgs{
	// 	Key:       key,
	// 	Value:     value,
	// 	Op:        op,
	// 	RequestId: ck.RequestId,
	// 	ClientId:  ck.ClientId,
	// }
	// args.Key = key
	// args.Value = value
	// args.Op = op
	args := GetPutAppendArgs{
		Key:       key,
		Value:     value,
		OpType:    op,
		RequestId: ck.RequestId,
		ClientId:  ck.ClientId,
	}
	shard := key2shard(key)
	DPrintf("client [%d] send PutAppend args [%+v], shardId [%d]", ck.ClientId, args, shard)

	for {
		gid := ck.config.Shards[shard]
		if servers, ok := ck.config.Groups[gid]; ok {
			for si := 0; si < len(servers); si++ {
				srv := ck.make_end(servers[si])
				var reply PutAppendReply
				ok := srv.Call("ShardKV.PutAppend", &args, &reply)
				if ok && reply.Err == OK {
					DPrintf("client [%d] receive PutAppend resply success", ck.ClientId)
					ck.RequestId++
					return
				}
				if ok && reply.Err == ErrWrongGroup {
					DPrintf("client [%d] send PutAppend request to worong group %d", ck.ClientId, gid)
					break
				}
				// ... not ok, or ErrWrongLeader
			}
		}
		time.Sleep(100 * time.Millisecond)
		// ask controler for the latest configuration.
		ck.config = ck.sm.Query(-1)
		DPrintf("client [%d] update config [%+v]", ck.ClientId, ck.config)
	}
}

func (ck *Clerk) Put(key string, value string) {
	ck.PutAppend(key, value, "Put")
}
func (ck *Clerk) Append(key string, value string) {
	ck.PutAppend(key, value, "Append")
}
