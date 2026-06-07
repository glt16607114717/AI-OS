const { chromium } = require('playwright');
const fs = require('fs');
const path = require('path');

const CONFIG_PATH = path.join(__dirname, 'account_config.json');

function loadConfig() {
    const data = fs.readFileSync(CONFIG_PATH, 'utf8');
    return JSON.parse(data);
}

function saveConfig(config) {
    fs.writeFileSync(CONFIG_PATH, JSON.stringify(config, null, 2), 'utf8');
}

function checkAccountExists(config, env, username) {
    if (!config.environments[env]) {
        return false;
    }
    
    const accounts = config.environments[env].accounts;
    if (accounts[username]) {
        return username;
    }
    return false;
}

function saveAccount(config, env, username, password, desc = '自动收集') {
    if (!config.environments[env]) {
        throw new Error(`环境不存在: ${env}`);
    }
    
    const accounts = config.environments[env].accounts;
    
    accounts[username] = {
        username: username,
        password: password,
        desc: desc
    };
    
    saveConfig(config);
    return username;
}

function listAccounts(config) {
    console.log('\n【账号列表】\n');
    
    for (const [envKey, envData] of Object.entries(config.environments)) {
        console.log(`环境: ${envData.name} (${envKey})`);
        console.log(`地址: ${envData.url}`);
        
        const accounts = envData.accounts;
        const accountList = Object.entries(accounts);
        
        if (accountList.length === 0) {
            console.log('账号: 暂无账号\n');
        } else {
            console.log('账号:');
            for (const [username, account] of accountList) {
                console.log(`  ${username} (${account.desc})`);
            }
            console.log('');
        }
    }
}

async function login(env, accountKey, customUsername = null, customPassword = null) {
    const config = loadConfig();
    
    if (!config.environments[env]) {
        throw new Error(`环境不存在: ${env}，可选: ${Object.keys(config.environments).join(', ')}`);
    }
    
    const envConfig = config.environments[env];
    
    let username, password, accountDesc;
    let isCustomAccount = false;
    
    if (customUsername && customPassword) {
        username = customUsername;
        password = customPassword;
        accountDesc = '自定义账号';
        isCustomAccount = true;
    } else if (accountKey) {
        const accounts = envConfig.accounts;
        if (!accounts[accountKey]) {
            const accountNames = Object.keys(accounts);
            if (accountNames.length === 0) {
                throw new Error(`该环境暂无账号，请提供自定义账号密码或使用其他方式登录`);
            }
            throw new Error(`账号不存在: ${accountKey}，可选: ${accountNames.join(', ')}`);
        }
        
        const account = accounts[accountKey];
        username = account.username;
        password = account.password;
        accountDesc = account.desc;
        
        if (!username || !password) {
            throw new Error(`账号 ${accountKey} 未配置用户名或密码，请编辑 account_config.json`);
        }
    } else {
        throw new Error('请指定账号或提供自定义账号密码');
    }
    
    console.log(`\n【开始登录】`);
    console.log(`环境: ${envConfig.name} (${env})`);
    console.log(`账号: ${username} (${accountDesc})`);
    console.log(`地址: ${envConfig.url}\n`);
    
    const browser = await chromium.launch({
        headless: config.browserConfig.headless,
        slowMo: config.browserConfig.slowMo
    });
    
    const context = await browser.newContext();
    const page = await context.newPage();
    
    try {
        await page.goto(envConfig.url, { 
            waitUntil: 'networkidle',
            timeout: config.browserConfig.timeout
        });
        
        const selectors = config.selectors;
        
        await page.waitForSelector(selectors.accountInput, { 
            timeout: config.browserConfig.timeout 
        });
        
        await page.fill(selectors.accountInput, username);
        await page.fill(selectors.passwordInput, password);
        
        await page.click(selectors.loginButton);
        
        await page.waitForURL(selectors.successUrl, { 
            timeout: config.browserConfig.timeout 
        });
        
        console.log(`✅ 登录成功！\n`);
        
        if (isCustomAccount) {
            const existingKey = checkAccountExists(config, env, username);
            if (existingKey) {
                console.log(`ℹ️  账号已存在于配置文件中（${env} 环境），跳过保存\n`);
            } else {
                const savedKey = saveAccount(config, env, username, password, '从Bug单收集');
                console.log(`📝 账号已自动保存到配置文件（${env} 环境的 ${savedKey} 账号）\n`);
            }
        }
        
        console.log(`浏览器窗口已打开，请进行后续操作...\n`);
        
    } catch (error) {
        console.error(`❌ 登录失败: ${error.message}\n`);
        await browser.close();
        throw error;
    }
}

async function main() {
    const args = process.argv.slice(2);
    
    if (args.length === 0) {
        const config = loadConfig();
        listAccounts(config);
        console.log('使用方法:');
        console.log('  node auto_login.js <环境> <账号名>');
        console.log('  node auto_login.js <环境> <用户名> <密码>');
        console.log('');
        console.log('示例:');
        console.log('  node auto_login.js dev ca-admin      # 登录开发环境 ca-admin 账号');
        console.log('  node auto_login.js test zhangyk     # 登录测试环境 zhangyk 账号');
        console.log('  node auto_login.js dev newuser 123456  # 使用自定义账号密码（自动保存）');
        console.log('  node auto_login.js --list            # 查看账号列表');
        return;
    }
    
    if (args.includes('--list') || args.includes('-l')) {
        const config = loadConfig();
        listAccounts(config);
        return;
    }
    
    const env = args[0];
    const accountKey = args[1];
    const customUsername = args[2] || null;
    const customPassword = args[3] || null;
    
    await login(env, accountKey, customUsername, customPassword);
}

if (require.main === module) {
    main().catch(error => {
        console.error(error);
        process.exit(1);
    });
}

module.exports = {
    login,
    listAccounts,
<<<<<<<< HEAD:skills/rmp-login/scripts/auto_login.js
    loadConfig
};
========
    loadConfig,
    saveAccount,
    checkAccountExists
};
>>>>>>>> 92b1c12d22fd34f132d62bd1c62903e31ec4108b:skills/common-rmp-web-login/scripts/auto_login.js
