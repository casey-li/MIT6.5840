# 运行脚本循环跑测试用例

for ((i=1;i<=500;i++))
do 
    result_file="./test_result/result_2C.txt"
    echo -e "Running the $i tests iteration" >> $result_file
    go test -run 2C -race >> $result_file
    echo " "  >> $result_file
done