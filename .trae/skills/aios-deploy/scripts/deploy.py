import subprocess
import os
import sys

def run_command(args, timeout=60):
    try:
        result = subprocess.run(
            args,
            capture_output=True,
            timeout=timeout
        )
        
        try:
            stdout = result.stdout.decode('utf-8').strip()
        except:
            stdout = result.stdout.decode('gbk', errors='ignore').strip()
        
        try:
            stderr = result.stderr.decode('utf-8').strip()
        except:
            stderr = result.stderr.decode('gbk', errors='ignore').strip()
        
        return result.returncode, stdout, stderr
    except subprocess.TimeoutExpired:
        return -1, "", "命令超时"

def main():
    action = 'deploy'
    if len(sys.argv) > 1:
        action = sys.argv[1].lower()
    
    # 基于脚本位置自动推导项目根目录（跨机器兼容）
    script_dir = os.path.dirname(os.path.abspath(__file__))
    # scripts/ -> aios-deploy/ -> skills/ -> .trae/ -> 项目根目录
    project_root = os.path.abspath(os.path.join(script_dir, '..', '..', '..', '..'))
    go_dir = os.path.join(project_root, 'go-backend')
    output_file = 'ai-os-server'
    server_host = 'ubuntu@124.221.220.89'
    deploy_port = '18731'
    
    print(f"=== AI-OS Go 后端部署 ===")
    print(f"操作类型: {action}")
    print(f"项目根目录: {project_root}")
    print(f"Go 目录: {go_dir}")
    
    if action in ['build', 'deploy']:
        os.chdir(go_dir)
        
        print("\n1. 交叉编译 Go 代码...")
        # 直接设置环境变量，确保 subprocess 继承
        os.environ['GOOS'] = 'linux'
        os.environ['GOARCH'] = 'amd64'
        
        returncode, stdout, stderr = run_command(['go', 'build', '-o', output_file, '.'], timeout=120)
        
        if stdout:
            print("STDOUT:", stdout)
        if stderr:
            print("STDERR:", stderr)
        
        if returncode != 0:
            print(f"编译失败，退出码: {returncode}")
            return
        
        print("编译成功")
        print(f"二进制位置: {os.path.join(go_dir, output_file)}")
    
    if action == 'deploy':
        print("\n2. 上传到服务器...")
        local_file = os.path.join(go_dir, output_file)
        remote_path = server_host + ':/home/ubuntu/aios-server/'
        
        returncode, stdout, stderr = run_command(['scp', local_file, remote_path], timeout=60)
        
        if returncode != 0:
            print(f"上传失败: {stderr}")
            return
        
        print("上传成功")
        
        print("\n3. 停止现有服务并释放端口...")
        stop_script = f"""
sudo systemctl stop ai-os
sleep 2
sudo kill -9 $(sudo lsof -ti:{deploy_port}) 2>/dev/null
sleep 1
"""
        returncode, stdout, stderr = run_command(['ssh', server_host, stop_script], timeout=30)
        print("STDOUT:", stdout)
        if stderr:
            print("STDERR:", stderr)
        
        print("\n4. 部署到生产目录...")
        deploy_script = f"""
echo "备份旧版本..."
sudo cp /opt/ai-os/{output_file} /opt/ai-os/{output_file}.bak 2>/dev/null

echo "复制新代码到生产目录..."
sudo cp /home/ubuntu/aios-server/{output_file} /opt/ai-os/{output_file}
sudo chmod +x /opt/ai-os/{output_file}

echo "启动服务..."
sudo systemctl start ai-os
sleep 3

echo "检查服务状态..."
sudo systemctl status ai-os
"""
        
        returncode, stdout, stderr = run_command(['ssh', server_host, deploy_script], timeout=60)
        
        print("STDOUT:", stdout)
        if stderr:
            print("STDERR:", stderr)
        
        if returncode == 0 and "Active: active" in stdout:
            print("\n=== 部署完成 ===")
        else:
            print(f"\n=== 部署失败 ===")
            print(f"退出码: {returncode}")
    
    elif action == 'restart':
        print("\n1. 停止服务...")
        returncode, stdout, stderr = run_command(['ssh', server_host, 'sudo systemctl stop ai-os'], timeout=30)
        print("STDOUT:", stdout)
        
        print("\n2. 释放端口...")
        kill_cmd = f"sudo lsof -ti:{deploy_port} | xargs -r sudo kill -9 2>/dev/null; sleep 1"
        returncode, stdout, stderr = run_command(['ssh', server_host, kill_cmd], timeout=30)
        print("端口释放完成")
        
        print("\n3. 启动服务...")
        returncode, stdout, stderr = run_command(
            ['ssh', server_host, 'sudo systemctl start ai-os && sleep 3 && sudo systemctl status ai-os'],
            timeout=30
        )
        
        print("STDOUT:", stdout)
        if stderr:
            print("STDERR:", stderr)

if __name__ == "__main__":
    main()
