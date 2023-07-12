package kvraft

import (
	"crypto/rand"
	"math/big"

	"6.5840/labrpc"
)

// 要求保存 leader，不重复处理命令
type Clerk struct {
	servers []*labrpc.ClientEnd
	// You will have to modify this struct.
	ClientId  int64
	RequestId int64
	LeaderId  int
}

func nrand() int64 {
	max := big.NewInt(int64(1) << 62)
	bigx, _ := rand.Int(rand.Reader, max)
	x := bigx.Int64()
	return x
}

func MakeClerk(servers []*labrpc.ClientEnd) *Clerk {
	ck := new(Clerk)
	ck.servers = servers
	// You'll have to add code here.
	ck.ClientId = nrand()
	ck.LeaderId = 0
	ck.RequestId = 0
	return ck
}

// fetch the current value for a key.
// returns "" if the key does not exist.
// keeps trying forever in the face of all other errors.
//
// you can send an RPC with code like this:
// ok := ck.servers[i].Call("KVServer.Get", &args, &reply)
//
// the types of args and reply (including whether they are pointers)
// must match the declared types of the RPC handler function's
// arguments. and reply must be passed as a pointer.
func (ck *Clerk) Get(key string) string {

	// You will have to modify this function.
	args := GetArgs{
		Key:       key,
		RequestId: ck.RequestId,
		ClientId:  ck.ClientId,
	}
	reply := GetReply{}
	for {
		DPrintf("[%d] send Get RPC [%v]", ck.ClientId, args)
		if ck.servers[ck.LeaderId].Call("KVServer.Get", &args, &reply) {
			DPrintf("[%d] receive Get reply [%v]", ck.ClientId, reply)
			switch reply.Err {
			case ErrWrongLeader:
				ck.LeaderId = (ck.LeaderId + 1) % len(ck.servers)
			case ErrNoKey, OK:
				ck.RequestId += 1
				return reply.Value
			}
		} else {
			DPrintf("[%d] call failed, leader changes from %d to %d", ck.ClientId, ck.LeaderId, (ck.LeaderId+1)%len(ck.servers))
			ck.LeaderId = (ck.LeaderId + 1) % len(ck.servers)
		}
	}
	// return ""
}

// shared by Put and Append.
//
// you can send an RPC with code like this:
// ok := ck.servers[i].Call("KVServer.PutAppend", &args, &reply)
//
// the types of args and reply (including whether they are pointers)
// must match the declared types of the RPC handler function's
// arguments. and reply must be passed as a pointer.
func (ck *Clerk) PutAppend(key string, value string, op string) {
	// You will have to modify this function.
	args := PutAppendArgs{
		Key:       key,
		Value:     value,
		Op:        op,
		RequestId: ck.RequestId,
		ClientId:  ck.ClientId,
	}
	reply := PutAppendReply{}
	for {
		DPrintf("[%d] send PutAppend RPC [%v]", ck.ClientId, args)
		if ck.servers[ck.LeaderId].Call("KVServer.PutAppend", &args, &reply) {
			DPrintf("[%d] receive PutAppend reply [%v]", ck.ClientId, reply)
			switch reply.Err {
			case ErrWrongLeader:
				ck.LeaderId = (ck.LeaderId + 1) % len(ck.servers)
			case ErrNoKey, OK:
				ck.RequestId += 1
				return
			}
		} else {
			DPrintf("[%d] call failed, leader changes from %d to %d", ck.ClientId, ck.LeaderId, (ck.LeaderId+1)%len(ck.servers))
			ck.LeaderId = (ck.LeaderId + 1) % len(ck.servers)
		}
	}
}

func (ck *Clerk) Put(key string, value string) {
	ck.PutAppend(key, value, "Put")
}
func (ck *Clerk) Append(key string, value string) {
	ck.PutAppend(key, value, "Append")
}
