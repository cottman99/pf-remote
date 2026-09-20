# PF Remote

PF Remote 是面向 AI 增强型技术个人的自托管计算资源网络。它把每台电脑
及其 Shell/Desktop 能力变成稳定、可授权、可复制给 Agent 的目标；用户不
需要理解 IP、端口、网关或底层协议。

本仓库是与个人生产部署完全分离的新核心。当前阶段只建设契约、可运行骨架
和旁路兼容能力，不替换任何正在工作的远程入口。

开始工作前请依次阅读：

1. [AGENTS.md](AGENTS.md)
2. [PROJECT_BRIEF.md](PROJECT_BRIEF.md)
3. [产品定义](docs/product/PRODUCT.md)
4. [架构](docs/architecture/ARCHITECTURE.md)
5. [威胁模型](docs/security/THREAT_MODEL.md)
6. [迁移边界](docs/migration/MIGRATION.md)

本地完整检查：

```powershell
./scripts/check.ps1
```
