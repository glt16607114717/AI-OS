import sys
import mysql.connector
from mysql.connector import Error

def main():
    if len(sys.argv) < 2:
        print("用法: python mysql_query.py \"SQL语句\"")
        return
    
    query = sys.argv[1]
    
    try:
        connection = mysql.connector.connect(
            host='124.221.220.89',
            port=23306,
            database='ai_os',
            user='root',
            password='glt01054717@'
        )
        
        if connection.is_connected():
            cursor = connection.cursor()
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
            
            # 如果是更新/插入/删除，提交事务
            query_upper = query.strip().upper()
            if query_upper.startswith('INSERT') or query_upper.startswith('UPDATE') or query_upper.startswith('DELETE'):
                connection.commit()
                print("事务已提交")
            
    except Error as e:
        print(f"数据库错误: {e}")
    finally:
        if connection.is_connected():
            cursor.close()
            connection.close()

if __name__ == "__main__":
    main()
