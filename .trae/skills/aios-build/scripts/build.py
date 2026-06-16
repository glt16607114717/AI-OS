import subprocess
import os

def main():
    client_dir = r'd:\wwwroot\AI\AI-OS\client'
    
    if not os.path.exists(client_dir):
        print(f"错误：客户端目录不存在: {client_dir}")
        return
    
    os.chdir(client_dir)
    
    print("=== AI-OS 客户端打包 ===")
    print("正在执行 npm run pack...")
    
    result = subprocess.run(
        'npm run pack',
        capture_output=True,
        text=True,
        timeout=600,
        shell=True
    )
    
    if result.stdout:
        print(result.stdout)
    
    if result.stderr:
        print("STDERR:", result.stderr)
    
    if result.returncode == 0:
        print("\n=== 打包成功 ===")
        print("安装包位置: D:\\ai-os-build\\")
    else:
        print(f"\n=== 打包失败 ===")
        print(f"退出码: {result.returncode}")

if __name__ == "__main__":
    main()
