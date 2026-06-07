# ADR-001: AI-OS 智能体协作网络架构决策

- **状态**：Proposed
- **日期**：2026-06-07
- **决策者**：桂良涛（后端负责人）
- **关联**：`docs/architecture.md` v3、`client/src/views/AgentNetwork.vue`、`server/app/agents/`、`server/app/graph/`、`server/app/privacy/`

---

## 背景

AI-OS 正从"本地 LLM 代理工具"向"企业级 AI 资产操作系统"演进。核心诉求是让多个 Agent 能力互补、互信协作，形成"1+1>2"的网络效应。

当前系统存在以下现实约束：
- AgentHub 是纯内存单例，进程重启即丢失全部网络状态
- Skills/Capability 模块仅有 proto 和启动器骨架，无运行时实现
- 三层存储（SQLite+Qdrant+Redis）全部直调，无仓储抽象，切换成本极高
- 自研算法（神经直觉引擎、概念隧道等）硬编码在 `private_graph.py` 中，无法热替换
- 前端已实现完整的 AgentNetwork 可视化（节点、连线、气泡消息），但后端无对应实现

本文档记录支撑智能体协作网络的 5 项核心架构决策。

---

## 决策 1：网络拓扑管理——中心化 vs 去中心化

### 问题
AgentHub 如何管理 Agent 节点的注册、发现和连接状态？

### 选项

| 选项 | 描述 | 优势 | 风险 |
|------|------|------|------|
| **A. 中心化 Hub（当前）** | AgentHub 内存字典 `dict[str, AgentNode]`，进程内管理 | 实现简单、调试方便、数据一致性强 | 单点故障、水平扩展受限 |
| B. 去中心化 P2P | Gossip 协议 + 本地 Agent 表 | 天然高可用 | 实现复杂、一致性难保证 |
| C. 混合模式 | Hub 管理元数据 + P2P 直连通信 | 兼顾两者 | 架构复杂度翻倍 |

### 决策

**选择 A，演进路线：A → C。**

Phase 1-2（当前~3个月）保持中心化 Hub，但为 Phase 3 的 P2P 扩展预留接口：
- AgentHub 抽象为 `IAgentRegistry` 接口（`register/unregister/discover`）
- 连接状态用事件总线广播（`AgentConnectedEvent`），未来可替换为 P2P 心跳
- 内存状态通过 `ExportHubState/ImportHubState` 与持久层双向同步

Phase 3 将 Hub 降级为"引导节点"，Agent 间直接通信走 P2P。

### 理由
当前团队规模（1-2 人）和系统体量（单机部署）下，中心化是最务实的选择。过早引入 P2P 会消耗 80% 精力在分布式一致性上。但接口抽象必须从第一天做，否则后期改造成本极高。

---

## 决策 2：能力组合模型——Slot 组合 vs Pipeline 编排 vs 图计算

### 问题
Agent 如何描述和组合自身能力？Skills 以什么方式组织？

### 选项

| 选项 | 描述 | 优势 | 风险 |
|------|------|------|------|
| **A. Slot 组合（当前设计）** | 每个 Agent 持有 4 个 Slot（文本/数据/图片/策略），Slot 可安装 Skill | 直觉、前端已实现 SkillSlot 可视化 | 组合爆炸、Skill 依赖管理难 |
| B. Pipeline 编排 | Agent 能力 = 一条 DAG 流水线 | 灵活、可调试 | 用户学习成本高 |
| C. 纯图计算 | 能力完全由连接权重动态涌现 | 最灵活 | 不可控、难解释 |

### 决策

**选择 A（Slot 组合），以策略层桥接图计算。**

- Agent 能力 = 4 个 SkillSlot 的组合态（`[s1, s2, s3, s4]`）
- Slot 通过 `MatchResult` 匹配最优 Skill（基于精确匹配、向量相似、规则三层）
- 连接策略层（Phase 2 图增强）可基于 Slot 匹配度动态调整连接权重
- Capability 模块（当前为空目录 `server/app/capability/`）需补充：
  - `capability/models.py` — SkillSlot、SlotType 枚举、MatchResult
  - `capability/matcher.py` — Slot 匹配引擎
  - `capability/repository.py` — 能力数据持久化

### 理由
Slot 模型与前端 AgentNetwork.vue 的 SkillSlot UI 完全对应，用户可以通过拖拽 Skill 到 Slot 来"训练"Agent。Pipeline 和图计算的收益在当前阶段无法体现（缺乏足够多的 Agent 和 Skill 数据），反而增加开发和理解成本。

**关键风险**：`server/app/capability/` 目前只有 `__init__.py`（4 行），proto 和 models 定义分散在 `agents/` 和 `capabilities/` 中，需要统一到此模块。

---

## 决策 3：隐私保护架构——四层分级 vs 开关式

### 问题
Agent 间协作时，如何保护隐私数据不被泄露？

### 选项

| 选项 | 描述 | 优势 | 风险 |
|------|------|------|------|
| **A. 四层分级（当前设计）** | LOW/MEDIUM/HIGH/MAXIMUM，每层对应不同脱敏策略 | 粒度细、可审计 | 配置复杂度高 |
| B. 开关式 | 全局开/关隐私保护 | 简单 | 粒度太粗，实际不可用 |
| C. 基于标签的 ABAC | 数据打标签，按标签策略自动保护 | 灵活 | 需要改造全链路数据模型 |

### 决策

**选择 A（四层分级），作为基线策略层，未来叠加 C。**

当前四层分级已有完整设计：
- `PrivacyLevel` 枚举 + `PRIVACY_PROTECT_MODEL` 映射表
- `MessagePrivacyGuard` 消息隐私守卫
- `PrivacyEncryptor` 配置加密器
- `SessionMemory`（三层记忆）的隐私感知检索

但存在以下缺口：
1. **CryptoEngine 引擎未实现**（`architecture.md` 中记录"待实现"）
2. 四层分级未与 AgentNetwork 连接策略联动（连接时应检查双方隐私等级兼容性）
3. 归因溯源（SimpleMKM 核心矩阵) 的隐私保护未实现

### 理由
四层分级是企业级场景的硬需求。用户必须能说"这个 Agent 处理的数据不离开本机"（MAXIMUM），而另一个可以"对外共享"（LOW）。开关式在这个场景下完全不可用。

---

## 决策 4：算法管理——硬编码 vs 算法工厂 vs 可插拔 Runtime

### 问题
自研算法（神经直觉引擎、概念隧道、MKM 归因等）如何管理和演进？

### 选项

| 选项 | 描述 | 优势 | 风险 |
|------|------|------|------|
| A. 硬编码（当前） | 算法直接写在 `private_graph.py` 中 | 开发快 | 无法 A/B 测试、无法热更新 |
| **B. 算法工厂 + 注册表** | AlgorithmRegistry 注册算法 → AlgorithmFactory 创建实例 → ABTestEngine A/B 测试 | 可控、可回退 | 中等复杂度 |
| C. 可插拔 Runtime | Rust 加速 + WASM 沙箱 + 热加载 | 极致性能 | 实现成本极高 |

### 决策

**选择 B（算法工厂），Phase 2 实施。**

Phase 1（当前）保持硬编码，但做两件准备工作：
1. 将 `private_graph.py` 中的算法函数提取为独立模块（每个算法一个文件）
2. 定义 `IAlgorithm` 接口（`analyze(graph) → result`）

Phase 2 实现：
- `AlgorithmRegistry` — 算法注册表，支持版本管理
- `AlgorithmFactory` — 工厂，按名称/版本创建算法实例
- `ABTestEngine` — A/B 测试引擎，按流量比例分配算法

### 理由
当前自研算法处于"验证期"，需要频繁调整参数和逻辑。硬编码是最快的迭代方式。但 architecture.md 明确规划了"系统 3.0 自研算法"路线（神经直觉引擎 → 图神经网络扩展 → 多模态融合），算法工厂是必经之路。

**关键发现**：`server/app/graph/private_graph.py`（1165 行）同时包含：
- 空间时序图注意力网络
- 认知不确定性量化
- 概念隧道发现
- MKM 核心矩阵归因
- 隐私图引擎

这些应该拆分为独立算法模块，否则维护成本会指数增长。

---

## 决策 5：存储架构——直调 vs 仓储模式 vs CQRS

### 问题
SQLite（结构化）、Qdrant（向量）、Redis（缓存）三层存储如何抽象？

### 选项

| 选项 | 描述 | 优势 | 风险 |
|------|------|------|------|
| A. 直调（当前） | 各模块直接调 `db.session.execute`、`qdrant_client.search` | 开发快 | 存储逻辑散落各处、无法统一切换 |
| **B. 仓储模式** | IStorage 接口 + Repository 实现 | 解耦、可测试、可切换 | 需要重构现有代码 |
| C. CQRS + Event Sourcing | 读写分离 + 事件流 | 极致扩展 | 过度设计 |

### 决策

**选择 B（仓储模式），分阶段重构。**

Phase 1（当前）保持直调，但在新模块中强制使用仓储接口。

Phase 2 重构优先级：
1. **记忆模块**（MemoryRepository）— 三层记忆已有清晰分层，改造成本最低
2. **智能体模块**（AgentRepository）— AgentHub 需要持久化，改造收益最高
3. **规则模块**（RuleRepository）— 规则引擎已有版本管理，改造风险最低
4. **图谱模块**（GraphRepository）— 与算法强耦合，最后改造

仓储接口草案：
```python
class IStorage(Protocol):
    def save(self, entity: T) -> str: ...
    def find_by_id(self, id: str) -> T | None: ...
    def find_by_filter(self, filter: dict) -> list[T]: ...
    def delete(self, id: str) -> bool: ...

class IMemoryStorage(IStorage):
    def search_by_vector(self, embedding: list[float], top_k: int) -> list[Memory]: ...
```

### 理由
当前直调方式在单机阶段够用，但 `architecture.md` 规划了"单机→集群→分布式"三阶段演进，Phase 2 就需要对接 PostgreSQL。如果不在 Phase 1 做好仓储抽象，Phase 2 的存储层改造会阻塞所有上层模块。

---

## 各决策间的关联

```
决策1（网络拓扑）──依赖──> 决策3（隐私保护）
    │                         │
    │                         │
    v                         v
决策2（能力组合）──依赖──> 决策5（存储架构）
    │
    v
决策4（算法管理）
```

- 决策 1 的 P2P 扩展依赖决策 3 的隐私保护（P2P 通信必须经过隐私守卫）
- 决策 2 的 Slot 组合需要决策 5 的仓储支撑（SkillSlot 需要持久化）
- 决策 4 的算法工厂依赖决策 5 的图谱仓储（算法输入是图数据）

---

## 立即行动项

| 优先级 | 行动 | 对应决策 | 负责人 |
|--------|------|----------|--------|
| P0 | 定义 `IAgentRegistry` 接口，AgentHub 实现之 | 决策 1 | 后端 |
| P0 | 补充 `capability/models.py`（SkillSlot、MatchResult） | 决策 2 | 后端 |
| P0 | CryptoEngine 基础实现（AES-256-GCM 密钥派生） | 决策 3 | 后端 |
| P1 | 拆分 `private_graph.py` 为独立算法模块 | 决策 4 | 后端 |
| P1 | 定义 `IStorage` 接口，MemoryRepository 首个实现 | 决策 5 | 后端 |
| P1 | 连接策略与隐私等级联动（连接时检查兼容性） | 决策 1+3 | 后端 |
| P2 | 算法工厂 + ABTestEngine | 决策 4 | 后端 |
| P2 | AgentRepository（AgentHub 持久化） | 决策 1+5 | 后端 |

---

## 风险登记

| 风险 | 影响 | 概率 | 缓解 |
|------|------|------|------|
| capability 模块空壳导致 Slot 功能无法落地 | 高 | 高 | P0 补充 models + matcher |
| AgentHub 内存态丢失（进程重启） | 中 | 高 | P0 定义 ExportHubState 持久化 |
| private_graph.py 1165 行无法维护 | 中 | 中 | P1 拆分为独立算法模块 |
| 四层隐私分级未与网络层集成 | 高 | 中 | P1 连接策略联动 |
| 存储直调导致 Phase 2 迁移阻塞 | 高 | 低（Phase 2 启动时） | P1 定义 IStorage 接口 |
