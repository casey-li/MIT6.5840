# 运行脚本循环跑测试用例

current_dir=$(pwd)

# 定义结果文件路径
result_file="$current_dir/Lab4_result.txt"

# for ((i=1;i<=250;i++))
# do 
#     echo -e "Running the $i tests iteration" >> "$result_file"
#     cd "$current_dir/shardctrler"
#     go test -race >> "$result_file"
#     echo " "  >> "$result_file"
#     cd "$current_dir/shardkv"
#     go test -race >> "$result_file"
#     echo " "  >> "$result_file"
# done

cd "$current_dir/shardctrler"
go test -race 
echo " " 
cd "$current_dir/shardkv"
go test -race 