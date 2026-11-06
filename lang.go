package main

import (
	"os"
	"strings"

	"github.com/egoist/mygo"
)

// The languages of the interface: English, and Simplified Chinese.
const (
	langEn = "en"
	langZh = "zh"
)

// langOf is "en" or "zh" for a locale such as "zh-CN", or "" for none.
// Traditional Chinese shows in English, the other language Zeus has.
func langOf(locale string) string {
	l := strings.ToLower(strings.TrimSpace(locale))
	switch {
	case l == "":
		return ""
	case strings.HasPrefix(l, "zh"):
		for _, part := range []string{"hant", "-tw", "_tw", "-hk", "_hk", "-mo", "_mo"} {
			if strings.Contains(l, part) {
				return langEn
			}
		}
		return langZh
	}
	return langEn
}

// systemLang is the language the system prefers, from the environment
// until the app is ready to name its locale itself, as in tests.
var systemLang string

func systemLanguage() string {
	if systemLang == "" {
		systemLang = langEn
		for _, v := range []string{"LC_ALL", "LANG"} {
			if l := langOf(os.Getenv(v)); l != "" {
				systemLang = l
				break
			}
		}
	}
	return systemLang
}

// resolveSystemLanguage replaces the environment's stand-in with the
// locale the running app reports. It runs once the app is ready, before
// the first view builds.
func resolveSystemLanguage() {
	if l := langOf(mygo.App.Locale()); l != "" {
		systemLang = l
	}
}

// uiLanguage is the language the interface shows in: the user's choice,
// else the system's.
func uiLanguage() string {
	switch prefs.Language {
	case langEn:
		return langEn
	case langZh:
		return langZh
	}
	return systemLanguage()
}

// zh are the Simplified Chinese texts, by their English; a text with no
// entry shows as it is, and so does everything in English.
var zh = map[string]string{
	// Commands.
	"Toggle File Tree":                   "切换文件树",
	"Close File Tree":                    "关闭文件树",
	"Files":                              "文件",
	"Copy Path":                          "复制路径",
	"Expand":                             "展开",
	"Collapse":                           "收起",
	"Refresh Folder":                     "刷新文件夹",
	"No active pane":                     "没有活动窗格",
	"Remote directory unknown":           "远端当前目录未知",
	"Remote host: %s":                    "远端主机：%s",
	"Browsing %s":                        "浏览位置：%s",
	"Manage shell integration":           "管理 Shell 集成",
	"SSH destination: %s":                "SSH 目标：%s",
	"Effective SSH host: %s":             "实际 SSH 主机：%s",
	"Remote user: %s":                    "远端用户：%s",
	"Checking remote shell integration…": "正在检查远端 Shell 集成…",
	"Install a %s hook in these remote startup files?":       "要在以下远端启动文件中安装 %s hook 吗？",
	"Remove the remote shell hook from these startup files?": "要从以下远端启动文件中移除 Shell hook 吗？",
	"Install shell hook":                         "安装 Shell hook",
	"Cancel shell hook setup":                    "取消安装",
	"Installing remote shell hook…":              "正在安装远端 Shell hook…",
	"Remote shell hook is installed":             "远端 Shell hook 已安装",
	"Reconnect SSH pane to apply the hook":       "重新连接 SSH 窗格以启用 hook",
	"Remove shell hook":                          "移除 Shell hook",
	"Confirm remove shell hook":                  "确认移除 Shell hook",
	"Cancel shell hook removal":                  "取消移除",
	"Removing remote shell hook…":                "正在移除远端 Shell hook…",
	"Shell hook removed; reconnect SSH to apply": "Shell hook 已移除；重新连接 SSH 以应用更改",
	"Retry shell hook setup":                     "重试 Shell hook 设置",
	"Unknown":                                    "未知",
	"Copied path":                                "路径已复制",
	"Loading…":                                   "加载中…",
	"This folder is empty":                       "此文件夹为空",
	"Retry":                                      "重试",
	"Folder: %s":                                 "文件夹：%s",
	"File: %s":                                   "文件：%s",
	"Symbolic link: %s":                          "符号链接：%s",
	"New Tab":                                    "新建标签页",
	"Split Right":                                "向右分屏",
	"Split Down":                                 "向下分屏",
	"Close Pane":                                 "关闭窗格",
	"Close Tab":                                  "关闭标签页",
	"Zoom Pane":                                  "放大窗格",
	"Equalize Panes":                             "平均分配窗格",
	"Next Tab":                                   "下一个标签页",
	"Previous Tab":                               "上一个标签页",
	"Rename Tab…":                                "重命名标签页…",
	"Command Palette…":                           "命令面板…",
	"Go to Pane…":                                "跳转到窗格…",
	"Find…":                                      "查找…",
	"Settings…":                                  "设置…",
	"Focus Pane Left":                            "聚焦左侧窗格",
	"Focus Pane Right":                           "聚焦右侧窗格",
	"Focus Pane Above":                           "聚焦上方窗格",
	"Focus Pane Below":                           "聚焦下方窗格",
	"Move Divider Left":                          "左移分隔线",
	"Move Divider Right":                         "右移分隔线",
	"Move Divider Up":                            "上移分隔线",
	"Move Divider Down":                          "下移分隔线",
	"Clear Screen and Scrollback":                "清空屏幕和回滚缓冲",
	"Clear Scrollback":                           "清空回滚缓冲",
	"Restart Shell in Pane":                      "重启窗格中的 Shell",
	"Bigger Text":                                "放大文字",
	"Smaller Text":                               "缩小文字",
	"Actual Size":                                "实际大小",
	"Appearance: Light":                          "外观：浅色",
	"Appearance: Dark":                           "外观：深色",
	"Appearance: System":                         "外观：跟随系统",
	"Quit and End All Sessions":                  "退出并结束所有会话",
	"Show Tab %d":                                "显示标签页 %d",

	// The menu bar.
	"Shell":      "Shell",
	"Edit":       "编辑",
	"View":       "显示",
	"Tabs":       "标签页",
	"Focus":      "聚焦",
	"Resize":     "调整大小",
	"Appearance": "外观",
	"Inspector":  "检查器",

	// The title bar and the panes.
	"Command Palette":  "命令面板",
	"Minimize":         "最小化",
	"Maximize":         "最大化",
	"Restore":          "还原",
	"Close":            "关闭",
	"Zoom":             "放大",
	"Unzoom":           "取消放大",
	"Divider":          "分隔线",
	"Needs attention":  "需要关注",
	"Printing":         "正在输出",
	"Running":          "正在运行",
	"Copied":           "已复制",
	"Dismiss":          "关闭",
	"More Tabs":        "更多标签页",
	"Reset Name":       "恢复默认名称",
	"Move Left":        "左移",
	"Move Right":       "右移",
	"Close Other Tabs": "关闭其他标签页",

	// The host's popover.
	"Host %s":                       "主机 %s",
	"Model":                         "型号",
	"Chip":                          "芯片",
	"Memory":                        "内存",
	"System":                        "系统",
	"User":                          "用户",
	"Server":                        "服务器",
	"Sessions":                      "会话",
	"pid %d · up %s":                "pid %d · 已运行 %s",
	"%d open, %d running a program": "%d 个会话，%d 个正在运行程序",
	"Quit":                          "退出",
	"and end all the sessions":      "并结束所有会话",
	"Sessions live in the server: quit Zeus and they keep running, and come back as they were when it opens again.": "会话保存在服务器中：退出 Zeus 后它们仍在运行，重新打开后会原样恢复。",
	"This Mac":      "这台 Mac",
	"This PC":       "这台电脑",
	"This computer": "这台电脑",

	// Find.
	"No matches":     "无匹配",
	"Navigate":       "选择",
	"Open":           "打开",
	"%d of %d":       "第 %d 个，共 %d 个",
	"Previous Match": "上一个匹配",
	"Next Match":     "下一个匹配",
	"Close Find":     "关闭查找",

	// The command palette.
	"Type a command, or the name of a pane…": "输入命令或窗格名称…",
	"Pane %d":          "窗格 %d",
	"Tab %d · Pane %d": "标签页 %d · 窗格 %d",

	// Programs and the sessions.
	"Git Changes":                 "Git 更改",
	"Git Log":                     "Git 历史",
	"Activity":                    "活动监控",
	"Pager":                       "分页器",
	"Could not start a session: ": "无法启动会话：",
	"%s finished":                 "%s 已结束",
	"in %s":                       "位于 %s",
	"Lost the connection to the session server.":                                                 "与 session 服务器的连接已断开。",
	"The session server is of another version of Zeus: quit and end all sessions to restart it.": "会话服务器属于另一个 Zeus 版本：请退出并结束所有会话后重启。",
	"Zeus could not start its session server":                                                    "Zeus 无法启动会话服务器",
	"Zeus could not restart its session server":                                                  "Zeus 无法重启会话服务器",

	// Settings.
	"Settings":                              "设置",
	"Terminal":                              "终端",
	"New tabs":                              "新建标签页",
	"Shortcuts":                             "快捷键",
	"Changes save automatically":            "更改会自动保存",
	"Language":                              "语言",
	"Choose the language of the interface.": "选择界面语言。",
	"Choose how Zeus looks and how terminal text is sized.":                         "选择 Zeus 的外观和终端文字的大小。",
	"Choose where a new tab starts. Split panes keep the focused pane's directory.": "选择新标签页的起始位置。分屏窗格始终沿用当前窗格的目录。",
	"Choose the cursor, the scrollback, and how selection copies.":                  "选择光标、回滚缓冲和选中复制的行为。",
	"Keyboard commands available in Zeus.":                                          "Zeus 中可用的键盘命令。",
	"Follow the system appearance or choose light or dark.":                         "跟随系统外观，或选择浅色、深色。",
	"Light":                       "浅色",
	"Dark":                        "深色",
	"Terminal text size":          "终端文字大小",
	"Applies to every open pane.": "应用于所有打开的窗格。",
	"Reset":                       "重置",
	"Font":                        "字体",
	"The family of the terminals' text, from the system's fonts or the one Zeus ships.": "终端文字的字体，可选系统字体或 Zeus 自带字体。",
	"Line height": "行高",
	"How tall a line of text is against the font's own.": "文字行高相对于字体默认行高的比例。",
	"Preview": "预览",
	"Cursor":  "光标",
	"The shape of the cursor, which programs may change.": "光标的形状，程序可以临时更改。",
	"Block":     "方块",
	"Bar":       "竖线",
	"Underline": "下划线",
	"Blinking":  "闪烁",
	"Whether the cursor blinks, unless a program asks otherwise.": "光标是否闪烁，除非程序另有要求。",
	"Scrollback": "回滚缓冲",
	"How much output a session keeps above the screen.": "会话在屏幕上方保留多少输出。",
	"Unlimited":      "无限制",
	"Copy on select": "选中即复制",
	"Selected text goes to the clipboard as soon as it is made.": "选中的文字立即复制到剪贴板。",
	"Start in": "起始位置",
	"Follow the focused pane, open at your home directory, or pick a folder.": "跟随当前窗格、打开主目录，或选择一个文件夹。",
	"This pane":                     "当前窗格",
	"Home":                          "主目录",
	"Choose…":                       "选择…",
	"Folder":                        "文件夹",
	"New tabs open at this folder.": "新标签页在此文件夹打开。",
	"Browse…":                       "浏览…",
	"Shell executable":              "Shell 可执行文件",
	"Leave empty to use the system login shell. Applies to new sessions.": "留空则使用系统登录 Shell。仅对新建会话生效。",
	"System default": "系统默认",
	"Press the keys for “%s”, or Esc to leave it as it is.": "为“%s”按下新的组合键，或按 Esc 保持不变。",
	"Press keys…":                        "按下按键…",
	"Change":                             "更改",
	"That combination is the terminal's": "该组合键是终端的保留键",
	"Already used by %s":                 "已被 %s 使用",
	"Choose a folder for new tabs":       "选择新标签页的文件夹",
}

// tr is a text of the interface, in the language it shows in.
func tr(s string) string {
	if uiLanguage() == langZh {
		if t, ok := zh[s]; ok {
			return t
		}
	}
	return s
}
