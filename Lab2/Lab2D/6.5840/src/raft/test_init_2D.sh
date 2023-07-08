# 运行脚本循环跑测试用例

for ((i=201;i<=400;i++))
do 
    # result_file="./test_result/result.txt"
    result_file="./log/${i}.txt"
    echo -e "Running the $i tests iteration" >> $result_file
    # go test -run 2A -race >> $result_file
    # go test -run 2B -race >> $result_file
    # go test -run 2C -race >> $result_file
    go test -run TestSnapshotInstallUnCrash2D -race >> $result_file
    echo " "  >> $result_file
done