# 运行脚本循环跑测试用例

for ((i=1;i<=150;i++))
do 
    result_file="./test_result/result.txt"
    echo -e "Running the $i tests iteration" >> $result_file
    go test -run 2D -race >> $result_file
    echo " "  >> $result_file
done