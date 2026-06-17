import subprocess
import os

def main():
    # 基于脚本位置自动推导项目根目录（跨机器兼容）
    script_dir = os.path.dirname(os.path.abspath(__file__))
    # scripts/ -> aios-build/ -> skills/ -> .trae/ -> 项目根目录
    project_root = os.path.abspath(os.path.join(script_dir, '..', '..', '..', '..'))
    client_dir = os.path.join(project_root, 'client')
    
    if not os.path.exists(client_dir):
        print(f"错误：客户端目录不存在: {client_dir}")
        return
    
    os.chdir(client_dir)
    
    print("=== AI-OS 客户端打包 ===")
    print(f"客户端目录: {client_dir}")
    print("正在执行 npm run pack...")
    
    result = subprocess.run(
        'npm run pack',
        capture_output=True,
        timeout=600,
        shell=True
    )
    
    # 手动解码，兼容 UTF-8 和 GBK
    try:
        stdout = result.stdout.decode('utf-8')
    except:
        stdout = result.stdout.decode('gbk', errors='ignore')
    
    try:
        stderr = result.stderr.decode('utf-8')
    except:
        stderr = result.stderr.decode('gbk', errors='ignore')
    
    if stdout:
        print(stdout[-3000:])
    
    if stderr:
        print("STDERR:", stderr[-1000:])
    
    if result.returncode == 0:
        print("\n=== 打包成功 ===")
        print("安装包位置: D:\\ai-os-build\\")
    else:
        print(f"\n=== 打包失败 ===")
        print(f"退出码: {result.returncode}")

if __name__ == "__main__":
    main()
