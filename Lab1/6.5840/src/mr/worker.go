package mr

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io/ioutil"
	"log"
	"net/rpc"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

type ByKey []KeyValue

// for sorting by key.
func (a ByKey) Len() int           { return len(a) }
func (a ByKey) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByKey) Less(i, j int) bool { return a[i].Key < a[j].Key }

// Map functions return a slice of KeyValue.
type KeyValue struct {
	Key   string
	Value string
}

// use ihash(key) % NReduce to choose the reduce
// task number for each KeyValue emitted by Map.
func ihash(key string) int {
	h := fnv.New32a()
	h.Write([]byte(key))
	return int(h.Sum32() & 0x7fffffff)
}

// 基本是 mrsequential.go 中 map 部分的工作流程
func doMapTask(reply *TaskReply, mapf func(string, string) []KeyValue) {
	pathPrefix := ""
	fullPath := pathPrefix + reply.FileNameList[0]
	file, err := os.Open(fullPath)
	if err != nil {
		log.Fatalf("cannot open %v", fullPath)
	}
	content, err := ioutil.ReadAll(file)
	if err != nil {
		log.Fatalf("cannot read %v", fullPath)
	}
	file.Close()
	results := mapf(fullPath, string(content))
	var fileNameList []string
	var tempFileNameList []string
	jsonList := createMapFile(reply.NReduce, reply.WorkerId, &fileNameList, &tempFileNameList)
	for _, kv := range results {
		index := ihash(kv.Key) % reply.NReduce
		jsonList[index].Encode(kv)
	}
	deleteTempFile(&fileNameList, &tempFileNameList)
	finishWork(reply.TaskType, reply.TaskId, fileNameList)
}

func createMapFile(nReduce int, workerId int, fileNameList *[]string, tempFileNameList *[]string) []*json.Encoder {
	var jsonList []*json.Encoder
	for i := 0; i < nReduce; i++ {
		var build strings.Builder
		build.WriteString("rm-")
		build.WriteString(strconv.Itoa(workerId))
		build.WriteString("-")
		build.WriteString(strconv.Itoa(i))
		fileName := build.String()
		f, _ := ioutil.TempFile("./", fileName)
		*fileNameList = append(*fileNameList, fileName)
		*tempFileNameList = append(*tempFileNameList, f.Name())
		jsonList = append(jsonList, json.NewEncoder(f))
	}
	return jsonList
}

func deleteTempFile(fileNameList *[]string, tempFileNameList *[]string) {
	for index, _ := range *fileNameList {
		err := os.Rename((*tempFileNameList)[index], (*fileNameList)[index])
		if err != nil {
			log.Fatalf("file rename failed, err: %v", err)
		}
	}
}

// 主要是 mrsequential.go 里的 reduce 部分的工作流程
func doReduceTask(reply *TaskReply, reducef func(string, []string) string) {
	kvList := reduceReadFile(reply.FileNameList)
	outputName := "mr-out-" + strconv.Itoa(reply.TaskId)
	outputfile, _ := ioutil.TempFile("./", outputName)
	i := 0
	for i < len(kvList) {
		j := i + 1
		for j < len(kvList) && kvList[j].Key == kvList[i].Key {
			j++
		}
		values := []string{}
		for k := i; k < j; k++ {
			values = append(values, kvList[k].Value)
		}
		output := reducef(kvList[i].Key, values)

		// this is the correct format for each line of Reduce output.
		fmt.Fprintf(outputfile, "%v %v\n", kvList[i].Key, output)
		i = j
	}
	outputfile.Close()
	err := os.Rename(outputfile.Name(), outputName)
	if err != nil {
		log.Fatalf("file rename failed, err: %v", err)
	}
	finishWork(reply.TaskType, reply.TaskId, reply.FileNameList)
}

func finishWork(taskType TaskTypes, taskId int, filePath []string) {
	args := TaskArgs{
		TaskId:      taskId,
		TaskType:    taskType,
		MessageType: TaskFinish,
		FilePath:    filePath,
	}
	reply := TaskReply{}
	call("Coordinator.ProcessTask", &args, &reply)
	// ok := call("Coordinator.ProcessTask", &args, &reply)
	// if !ok {
	// 	fmt.Println("call failed !!")
	// }
}

// reduce 读取键值对并对键值对进行排序
func reduceReadFile(fileNameList []string) []KeyValue {
	filePathPrefix := ""
	kva := []KeyValue{}
	for _, filename := range fileNameList {
		fullPath := filePathPrefix + filename
		file, err := os.Open(fullPath)
		if err != nil {
			return kva
		}
		dec := json.NewDecoder(file)
		for {
			var kv KeyValue
			if err := dec.Decode(&kv); err != nil {
				break
			}
			kva = append(kva, kv)
		}
	}
	sort.Sort(ByKey(kva))
	return kva
}

// main/mrworker.go calls this function.
// 调用的工作函数，传入的 map 函数和 reduce 函数
func Worker(mapf func(string, string) []KeyValue,
	reducef func(string, []string) string) {

	// Your worker implementation here.
	// fmt.Println("Worker init")

	for {
		reply, ok := RequestTask()
		if !ok {
			break
		}
		if reply.WorkerId != 0 {
			taskType := reply.TaskType
			switch taskType {
			case MapTask:
				doMapTask(&reply, mapf)
			case ReduceTask:
				doReduceTask(&reply, reducef)
			case Wait:
				time.Sleep(2 * time.Second)
			}
		}
	}
	// uncomment to send the Example RPC to the coordinator.
	// CallExample()

}

// 请求任务，即修改下面的 CallExample()
func RequestTask() (TaskReply, bool) {
	args := TaskArgs{}
	reply := TaskReply{}
	args.MessageType = TaskRequest
	ok := call("Coordinator.ProcessTask", &args, &reply)
	// fmt.Println(reply)
	if ok {
		return reply, ok
	} else {
		// fmt.Println("call failed !!")
		return TaskReply{}, ok
	}
}

// example function to show how to make an RPC call to the coordinator.
// the RPC argument and reply types are defined in rpc.go.
func CallExample() {

	// declare an argument structure.
	args := ExampleArgs{}

	// fill in the argument(s).
	args.X = 99

	// declare a reply structure.
	reply := ExampleReply{}

	// send the RPC request, wait for the reply.
	// the "Coordinator.Example" tells the
	// receiving server that we'd like to call
	// the Example() method of struct Coordinator.
	ok := call("Coordinator.Example", &args, &reply)
	if ok {
		// reply.Y should be 100.
		fmt.Printf("reply.Y %v\n", reply.Y)
	} else {
		fmt.Printf("call failed!\n")
	}
}

// send an RPC request to the coordinator, wait for the response.
// usually returns true.
// returns false if something goes wrong.
func call(rpcname string, args interface{}, reply interface{}) bool {
	// c, err := rpc.DialHTTP("tcp", "127.0.0.1"+":1234")
	sockname := coordinatorSock()
	c, err := rpc.DialHTTP("unix", sockname)
	if err != nil {
		// log.Fatal("dialing:", err)
		return false
	}
	defer c.Close()

	err = c.Call(rpcname, args, reply)
	if err == nil {
		return true
	}

	// fmt.Println(err)
	return false
}
