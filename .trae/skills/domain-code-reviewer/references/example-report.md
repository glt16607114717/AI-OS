# 示例审查报告

以下为算法岗 PyTorch 训练代码的完整审查报告示例，展示报告结构和输出规范。

---

## 审查过程（thinking）

分析代码...

1. **维度注释**：
   - L23: `x = self.fc(x)` - 输入 x 的维度未注释
   - 根据 `user.rules > 代码规范 > 第1条`，这是违规

2. **内存管理**：
   - L45-52: 训练循环中未清理计算图
   - 累积 loss 会导致 OOM

3. **数值稳定性**：
   - L38: `loss = -torch.log(prob)` - prob 可能为 0，导致 log(0)
   - 应使用 `torch.log(prob + eps)` 或 `torch.log_softmax`

---

## 审查报告

# 📋 领域驱动代码审查报告

> **审查上下文**：AI/ML / PyTorch / train.py

## 🚨 领域规则红线

### Violation #1: 缺少 Tensor 维度注释

**违规位置**：`train.py:L23-L25`

**规则来源**：`user.rules > 代码规范 > 第1条：所有 Tensor 操作必须注明维度`

**当前代码**：
```python
x = self.fc(x)
x = F.relu(x)
return x
```

**修复方案**：
```python
# [B, 768] -> [B, 256]
x = self.fc(x)
x = F.relu(x)
return x
```

### Violation #2: 未处理 log(0) 风险

**违规位置**：`train.py:L38`

**规则来源**：`user.rules > 数值稳定性 > 第2条：禁止未保护的 log 运算`

**当前代码**：
```python
loss = -torch.log(prob)
```

**修复方案**：
```python
# [!]: 防止 log(0) 导致 NaN
eps = 1e-8
loss = -torch.log(prob + eps)
# 或更优方案：使用 log_softmax
loss = F.nll_loss(F.log_softmax(logits, dim=-1), targets)
```

## 🛠️ 工程质量隐患

### Issue #1: 训练循环内存累积

**风险等级**：🔴 高

**影响范围**：性能 / 稳定性

**问题描述**：
L45-52 的训练循环中，`total_loss += loss` 会保留计算图，导致显存持续增长。

**建议方案**：
```python
# [!]: 使用 .item() 断开计算图
total_loss += loss.item()
```

## 💡 优化建议

### Suggestion #1: 使用梯度裁剪

**当前写法**：
```python
optimizer.step()
```

**推荐写法**：
```python
# [!]: 防止梯度爆炸
torch.nn.utils.clip_grad_norm_(model.parameters(), max_norm=1.0)
optimizer.step()
```

**理由**：算法岗常见最佳实践，提高训练稳定性

## ✅ 闪光点

- L15-20: 数据增强 pipeline 设计合理，符合 On-the-fly 增强最佳实践
- L55-60: Checkpoint 保存逻辑完整，包含 optimizer 状态

## 📊 审查总结

| 指标 | 数量 |
|-----|-----|
| 🔴 红线违规 | 2 |
| 🟡 质量隐患 | 1 |
| 💡 优化建议 | 1 |
| ✅ 闪光点 | 2 |

**综合评价**：核心逻辑正确，但缺少必要的防护措施。修复红线问题后可合入。
