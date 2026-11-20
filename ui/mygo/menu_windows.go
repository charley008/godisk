package desktop

import "github.com/egoist/mygo"

func (d *desktop) installMenu() {
	item := func(label, shortcut string, action func()) *mygo.MenuItem {
		return &mygo.MenuItem{Label: label, Accelerator: shortcut, Click: func(_ *mygo.MenuItem, w *mygo.Window) {
			w.Update(action)
		}}
	}
	d.themeMenu = &mygo.MenuItem{
		Label: "深色主题", Type: mygo.MenuItemCheckbox, Checked: d.darkTheme,
		Click: func(it *mygo.MenuItem, _ *mygo.Window) {
			d.setAppearance(it.IsChecked())
		},
	}
	d.win.OnClosed(mygo.Theme.OnUpdated(func() {
		d.win.Update(func() { d.syncAppearance(mygo.Theme.IsDark()) })
	}))
	d.win.SetMenu(mygo.NewMenu([]*mygo.MenuItem{
		{Label: "文件", Submenu: []*mygo.MenuItem{
			item("打开健康快照…", "Ctrl+O", func() { d.page = 0; d.importCapture() }),
			item("导出健康快照…", "Ctrl+S", func() { d.page = 0; d.exportCapture() }),
			mygo.Separator(),
			item("退出", "", d.win.Close),
		}},
		{Label: "查看", Submenu: []*mygo.MenuItem{
			item("磁盘健康", "Ctrl+1", func() { d.page = 0 }),
			item("性能测试", "Ctrl+2", func() { d.page = 1 }),
			item("历史记录", "Ctrl+3", func() { d.page = 2 }),
			mygo.Separator(),
			d.themeMenu,
		}},
		{Label: "测试", Submenu: []*mygo.MenuItem{
			item("开始测试", "", func() { d.page = 1; d.startBenchmark() }),
			item("取消测试", "", d.stopBenchmark),
			item("高级设置", "", func() { d.page = 1; d.advanced = !d.advanced }),
		}},
		{Label: "帮助", Submenu: []*mygo.MenuItem{
			item("关于 goDisk", "", func() {
				_, _ = mygo.Dialog.Message(mygo.MessageOptions{
					Parent: d.win, Type: mygo.MessageInfo, Title: "关于 goDisk",
					Message: "goDisk " + mygo.App.Version(),
					Detail:  "Disk Health & Performance Monitor\nWindows 磁盘健康检测与性能测试工具\n\n测速引擎：Microsoft DiskSpd 2.3",
				})
			}),
		}},
	}))
}
