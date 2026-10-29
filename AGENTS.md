# AGENTS.md

本文件为人类开发者/Coding Agent 所写，用于规范化本项目开发过程中的一些细节

Written for human developers and coding agents, to lay out the details of how this project is developed.

## 0. 先看什么 / Read first

1. `README.md` 与 `README_zh-CN.md` —— 项目说明及使用文档，可能会索引到其他文档，请一并阅读
2. `docs/*.md` —— 详细介绍文档，更新频次较快，故不在此作硬编码文件路径，请自行查找并查看
3. 涉及哪个模块的工作，先读那个文件开头的注释

本项目每个源文件开头均说明了其编写原因。请遵守第三条，不要凭猜测推断代码意图，也不要进行破坏性修改

1. `README.md` and `README_zh-CN.md` — project overview; may index other docs, read those too
2. `docs/*.md` — detailed documentation, updated too often to list here
3. Read the header comment of the module you are about to work on before touching it

Every source file explains at its top why it is written that way. Follow rule 3 above: do not infer intent
by guesswork, and do not make destructive changes.

### 1. 依赖引入 / Dependencies

第三方依赖的处理原则：**适当采用成熟依赖替代自行实现**

自行实现容易在边界条件上出错，而这类错误往往只在特定输入下才会暴露
成熟库的使用者众多，常见问题在大部分情况下都已被发现并修正

另外，引入依赖前应确认：

- 它消除的是**容易出错**的代码，还是仅减少了几行工作量？
- 它会引入多少传递依赖，体积与供应链风险是否可接受？
- 项目是否仍在维护，许可证是否允许当前用法？

简单来说：**依赖并非不可引入，但不应随意引入，引入前需权衡利弊**

Third-party dependencies: **prefer a mature dependency over a hand-rolled implementation**

Hand-rolled code tends to get edge cases wrong, and those errors often only surface under specific input.
A mature library has many users, so common problems have in most cases already been found and fixed.

Before adding a dependency, also confirm:

- Does it eliminate **error-prone** code, or merely save a few lines?
- How many transitive dependencies does it pull in, and are the size and supply-chain risk acceptable?
- Is the project still maintained, and does its licence permit the intended use?

In short: **a dependency is not forbidden, but it must not be added casually; weigh the trade-offs first**

### 2. 提交信息 / Commit messages

提交信息**用英文写**，遵循[约定式提交](https://www.conventionalcommits.org/)：
`<type>(<scope>): <description>`，**只写一行**，简洁有力地说明这次改了什么

英语是通用语言，使用英语作为提交信息可以覆盖更多人群，便于后期其他外部开发者做出贡献

不写多行小作文
提交信息只需要简洁、清晰明了的一行文本
提交信息的作用是让人知道"这一步做了什么"，而"为什么这么做"则应当写在代码注释里

常用 type：`feat` `fix` `refactor` `perf` `docs` `test` `build` `ci` `chore` `style`

```
feat(update): use the Semver library for version comparison
docs: document the .dev/ convention in AGENTS.md
```

Commit messages are **written in English** and follow
[Conventional Commits](https://www.conventionalcommits.org/):
`<type>(<scope>): <description>`, **on a single line**, stating concisely what changed.

English is the lingua franca.
So writing commit messages in English reaches a wider audience and makes it easier for other external developers to contribute later.

No multi-paragraph essays.
The commit message only needs to be a concise, clear one-liner.
Its job is to say "what this step did"; "why it was done" belongs in code comments.

Common types: `feat` `fix` `refactor` `perf` `docs` `test` `build` `ci` `chore` `style`

### 3. 测试数据与示例值 / Test data and fixtures

**代码与测试数据应该使用规范、清晰易懂的通用名称进行命名**

- 示例值一律用 `example` / `Example` / `EXAMPLE` / `examples` 这类**通用占位**，
  例如用户名用 `example`，域名用 `example.com`，仓库用 `username/repository`
- **不得**使用真实域名、仓库名、用户名、设备序列号或使用开发机器上的绝对路径来编写代码/测试用例/注释，也不得使用**由它们截取或变体而来的名字**

**Code and test data must be named with standard, clear, generic terms**

- Fixtures use generic placeholders such as `example` / `Example` / `EXAMPLE` / `examples` —
  for instance `example` for a user name, `example.com` for a domain and `username/repository`
  for a repository
- Do **not** write a real domain, repository name, user name or device serial into code, test cases or
  comments, and do not use an absolute path from a development machine — nor a name **derived or
  abbreviated** from any of them

## 4. 针对 Agent 的协作方式建议

### 4.1 修改与提交

- 分批修改：对用户下定的任务进行拆分，分多次进行小幅度的修改，适当多用 commit，避免一次性提交大量代码
- 小步提交与可维护性：除明确要求及特殊情况外，尽量让每次 commit 时的代码保持可编译并具有清晰易懂的 commit 消息，一批只做一件事，并明确修改范围，便于后续 review 与分析问题
- 未经允许不推送：可以适度提交，但推送分支、打 tag 与发布 Release 均由维护者决定，不要在未明确授权的情况下擅自推送或打 tag

### 4.2 沟通与透明

- 行为透明：禁止一味地思考而不向用户输出你正在执行的任务；在思考与分析或是连续执行任务期间适度地间歇性告知用户你正在思考/理解/修改的内容
- 先说明再修改：阐明目标与影响范围；涉及高风险范围或可能导致严重后果时，先给出方案，不要擅自执行
- 保持透明：在每批工作完成后报告已完成事项、验证结果、遗留风险
- 高准确性：存在不确定的问题时先进行确认，禁止猜测

### 4.3 注释与文案

- 简洁易懂：尽量使用简短、通俗易懂的语言风格来编写注释，以降低后期维护成本
- 维护注释：全部代码注释与文档需保持中英双语并遵循本规则；若变更代码内容需同步排查并更新对应注释
- 注释语言结构：注释须写成连续的整段中文 + 连续的整段英文，不得中英逐行交替
- 断言边界：注释、文案与说明中不得使用「永久」「永远」「绝不」「一定会」这类无限定范围的断言，也不得把当前观察到的情况说成普遍规律；写清边界：在什么条件下、哪个函数走哪条分支、因此不会出现什么结果

### 4.4 格式与多语言

- 保持格式：不要在句尾添加句号，除非一行有两个句子需要区分；也不要在行尾添加没有必要的句号；如需编写多行注释，保证一句话在一行注释内写完，严禁一句话分多行来写
- 多语言：修改日志 / UI 等面向用户的内容必须同步更新全部 i18n 文件并进行验证

### 4.5 文件与禁止事项

- 临时文件一律放 `.dev/`：此目录已被 git 忽略，用于存放临时文件；一切不应被 git 追踪的文件均应该放置在此目录，如临时排查脚本、测试用配置等
- 严禁修改 `AGENTS.md` 文件