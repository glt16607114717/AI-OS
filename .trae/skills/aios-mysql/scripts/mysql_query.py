import sys
import pymysql
import os

def main():
    if len(sys.argv) < 2:
        print("用法: python mysql_query.py \"SQL语句\" 或 python mysql_query.py --file sql文件路径")
        return
    
    query = ""
    
    # 检查是否是文件输入
    if sys.argv[1] == "--file" and len(sys.argv) >= 3:
        file_path = sys.argv[2]
        try:
            with open(file_path, 'r', encoding='utf-8') as f:
                query = f.read().strip()
        except Exception as e:
            print(f"读取文件失败: {e}")
            return
    else:
        query = sys.argv[1]
    
    try:
        connection = pymysql.connect(
            host='8.163.127.182',
            port=3306,
            database='ai_os',
            user='root',
            password='glt01054717@',
            charset='utf8mb4'
        )
        
        with connection.cursor() as cursor:
            cursor.execute(query)
            
            # 获取列名
            columns = [col[0] for col in cursor.description] if cursor.description else []
            
            # 获取结果
            results = cursor.fetchall()
            
            print("=== 查询结果 ===")
            
            if columns:
                print(" | ".join(columns))
                print("-" * (len(" | ".join(columns))))
            
            for row in results:
                row_str = []
                for val in row:
                    if val is None:
                        row_str.append("NULL")
                    else:
                        row_str.append(str(val))
                print(" | ".join(row_str))
            
            print(f"\n共 {len(results)} 条记录")
            
            # 如果是更新/插入/删除/CREATE，提交事务
            query_upper = query.strip().upper()
            if query_upper.startswith('INSERT') or query_upper.startswith('UPDATE') or query_upper.startswith('DELETE') or query_upper.startswith('CREATE') or query_upper.startswith('ALTER') or query_upper.startswith('DROP'):
                connection.commit()
                print("执行成功")
                
    except Exception as e:
        print(f"数据库错误: {e}")
    finally:
        if 'connection' in locals():
            connection.close()

if __name__ == "__main__":
    main()