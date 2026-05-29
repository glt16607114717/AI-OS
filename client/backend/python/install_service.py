"""
AI-OS Agent service installer.
Called by NSIS during install/uninstall.
Usage:
    python install_service.py install   -- register and start service
    python install_service.py uninstall -- stop and remove service
"""
import os
import sys
import subprocess
import xml.etree.ElementTree as ET


def get_install_dir():
    return os.path.dirname(os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__)))))


def get_winsw_dir(install_dir):
    return os.path.join(install_dir, "resources", "winsw")


def find_python():
    return sys.executable


def generate_xml(winsw_dir, python_exe, agent_script):
    xml_path = os.path.join(winsw_dir, "ai-os-agent.xml")
    root = ET.Element("service")
    ET.SubElement(root, "id").text = "AI-OS-Agent"
    ET.SubElement(root, "name").text = "AI-OS Agent"
    ET.SubElement(root, "description").text = "AI-OS Background Agent Service"
    ET.SubElement(root, "executable").text = python_exe
    ET.SubElement(root, "arguments").text = f'"{agent_script}"'
    ET.SubElement(root, "startmode").text = "Automatic"
    ET.SubElement(root, "onfailure", action="restart", delay="10 sec")
    ET.SubElement(root, "onfailure", action="restart", delay="30 sec")
    ET.SubElement(root, "onfailure", action="restart", delay="60 sec")
    ET.SubElement(root, "resetfailure").text = "1 hour"
    log_elem = ET.SubElement(root, "log", mode="roll-by-size")
    ET.SubElement(log_elem, "sizeThreshold").text = "10240"
    ET.SubElement(log_elem, "keepFiles").text = "8"
    tree = ET.ElementTree(root)
    ET.indent(tree, space="  ")
    tree.write(xml_path, encoding="unicode", xml_declaration=True)
    print(f"Generated: {xml_path}")
    return xml_path


def run_winsw(winsw_dir, *args):
    exe = os.path.join(winsw_dir, "ai-os-agent.exe")
    if not os.path.exists(exe):
        print(f"ERROR: WinSW not found: {exe}")
        return False
    cmd = [exe] + list(args)
    print(f"Running: {' '.join(cmd)}")
    result = subprocess.run(cmd, capture_output=True, text=True, timeout=30)
    if result.stdout:
        print(result.stdout)
    if result.stderr:
        print(result.stderr)
    return result.returncode == 0


def do_install():
    install_dir = get_install_dir()
    winsw_dir = get_winsw_dir(install_dir)
    agent_script = os.path.join(install_dir, "resources", "backend", "python", "agent", "main.py")
    python_exe = find_python()

    print(f"Install dir: {install_dir}")
    print(f"Python: {python_exe}")
    print(f"Agent: {agent_script}")

    generate_xml(winsw_dir, python_exe, agent_script)

    print("Installing service...")
    run_winsw(winsw_dir, "install")

    print("Starting service...")
    run_winsw(winsw_dir, "start")

    print("DONE: Service installed and started")


def do_uninstall():
    install_dir = get_install_dir()
    winsw_dir = get_winsw_dir(install_dir)

    print("Stopping service...")
    run_winsw(winsw_dir, "stop")

    print("Uninstalling service...")
    run_winsw(winsw_dir, "uninstall")

    print("DONE: Service stopped and removed")


if __name__ == "__main__":
    if len(sys.argv) < 2:
        print("Usage: python install_service.py [install|uninstall]")
        sys.exit(1)
    action = sys.argv[1]
    if action == "install":
        do_install()
    elif action == "uninstall":
        do_uninstall()
    else:
        print(f"Unknown action: {action}")
        sys.exit(1)
