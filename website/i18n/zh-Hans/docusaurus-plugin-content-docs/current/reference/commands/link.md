---
sidebar_position: 3
---

# link

把一个文件夹（例如你自己的 skills checkout）直接链接到 skills Source 之下。开启 [`follow_source_links`](../targets/configuration.md#follow_source_links) 后，skillshare 会列出并 sync 其中的 skills。

```bash
skillshare link ~/code/dev-skills                  # 以 _dev-skills 链接
skillshare link ~/code/dev-skills --enable         # 链接并开启跟随
skillshare link ~/code/dev-skills --name _mine     # 自定义链接名
skillshare link ../team --name _team -p            # 链接到项目的 Source
skillshare link -- -checkout                       # 链接以 - 开头的路径
```

## 何时使用

- 在 skills checkout 原来的位置继续编辑，而不是安装一份副本
- 使用[外置硬盘](../targets/configuration.md#skills-on-an-external-drive)上的 skills
- 在 Windows 上创建链接，无需自己在 junction 与 symlink 之间做选择

```text
$ skillshare link ~/code/dev-skills --enable
✓ Linked _dev-skills → ~/code/dev-skills · symlink
✓ Set follow_source_links: true ~/.config/skillshare/config.yaml

Next
  skillshare sync  link its skills into your targets
```

## 会发生什么

1. 链接名默认为 `_` 加文件夹名（`_dev-skills`），必须是直接位于 Source 之下的单个名称。
2. 按发现阶段检查被跟随链接的同样方式检查文件夹。如果名称已存在，或文件夹是 Source 本身或其上级、位于 Source 内部、与 sync target 重叠、不存在、不可读或不是目录，链接会被拒绝。
3. 创建链接：macOS 和 Linux 上是绝对路径的 symlink。Windows 上会创建 junction，不需要开发者模式或管理员权限；只有 junction 创建失败时才改试目录 symlink。
4. 文件夹没有 `.git` 条目时会给出警告：链接仍然可用，但 `skillshare update` 无法 pull 它。
5. `follow_source_links` 关闭时，除非加上 `--enable`，否则不会修改配置。不加时，命令会打印与 `skillshare doctor` 相同的提示。如果 `--enable` 无法保存配置，新建的链接会被移除、命令失败，不会留下未被跟随的链接。Dashboard 的 **Link folder** 复选框也一样。

## 选项

| Flag | Description |
|------|-------------|
| `--name <name>` | Link name in the source (default: `_<folder name>`) |
| `--enable` | Also set `follow_source_links: true` |
| `--project, -p` | Use project-level config in current directory |
| `--global, -g` | Use global config (`~/.config/skillshare`) |
| `--` | End options; allow a path starting with `-` |
| `--help, -h` | Show help |

## 不加 `--enable`

```text
$ skillshare link ~/code/dev-skills
✓ Linked _dev-skills → ~/code/dev-skills · symlink
! target is not a git checkout; skillshare update cannot pull it
  _dev-skills: not followed by discovery; its contents are invisible to skillshare. Set follow_source_links: true to follow it
```

链接已经创建，但在配置中设置 `follow_source_links: true` 之前，发现阶段会忽略它。

## 被拒绝的链接

命令以非零状态退出，且不创建任何内容：

```text
$ skillshare link ~/.claude
✗ cannot link /home/me/.claude: target overlaps sync target /home/me/.claude/skills
```

| 原因 | 成因 |
|--------|-------|
| `target is the source or a parent of it` | 跟随它会绕回 Source 本身 |
| `target is inside the source` | 其中的 skills 已经在 Source 里 |
| `target overlaps sync target <path>` | 文件夹就是、包含或位于某个启用中的 skills target 内 |
| `target is a link chain that could not be resolved` | 该路径是一个目标无法解析的链接 |
| `target is missing` / `target is not readable` / `target is unavailable: <error>` | 文件夹不存在或不可读 |
| `target is not a directory` | 该路径是文件 |
| `<name> already exists` | Source 中已有同名条目 |
| `"<name>" is not a first-level name` | `--name` 含有路径分隔符，或是 `.` / `..` |

## 手动链接

`skillshare link` 创建的条目与你自己创建的链接（`ln -s`，或 Windows 上的 `mklink /J`）相同，发现阶段对两者一视同仁。命令只是在创建之前先做上述检查。

## 另请参阅

- [unlink](./unlink.md) — 移除链接
- [Configuration — `follow_source_links`](../targets/configuration.md#follow_source_links) — 发现行为、安全防护与经由链接的写入
- [doctor](./doctor.md) — 报告每个 Source 链接的状态
