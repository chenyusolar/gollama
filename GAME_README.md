# 3D游戏演示说明

## 概述

这是一个使用纯Go编写的3D WebGL游戏演示,展示如何使用JavaScript和Canvas API创建3D图形。

## 功能特性

### 🎮 控制方式

- **WASD移动**: 
  - W - 前进
  - S - 后退
  - A - 向左
  - D - 向右

- **空格跳跃**: 按空格键跳跃,可以跳上地形

### 🎯 游戏目标

- 在3D地形中移动角色
- 收集散落的彩色立方体
- 靠近立方体即可自动收集

### 📊 实时显示

- FPS计数器:显示当前帧率
- 每秒更新一次

## 运行方式

### 方法1: 直接打开
1. 双击 `game3d.html` 文件
2. 使用浏览器打开HTML文件
3. 开始游戏

### 方法2: 通过HTTP服务器
```bash
# 在项目根目录
cd e:/ollm-project/gollama

# 使用Python启动简单HTTP服务器
python -m http.server 8000

# 然后浏览器访问
http://localhost:8000/game3d.html
```

### 方法3: 使用Live Server
```bash
# 如果有VS Code Live Server扩展
# 右键点击game3d.html
# 选择"Open with Live Server"
```

## 技术细节

### WebGL特性
- 使用顶点和片元着色器
- 透视投影矩阵
- 深度测试
- 立方体和地形渲染

### 游戏机制
- 简单的物理模拟(重力、跳跃)
- 地形碰撞检测
- 收集物品系统
- 实时FPS计算

## 自定义

### 修改地形
在 `Game3D.createTerrain()` 方法中可以修改地形生成:

```javascript
createTerrain() {
    // 修改这些值来改变地形
    for (let x = -10; x <= 10; x++) {
        for (let z = -10; z <= 10; z++) {
            const y = Math.sin(x * 0.3) * Math.cos(z * 0.3) * 2;
            this.terrain.push({ x, y, z });
        }
    }
}
```

### 修改立方体
在 `Game3D.createCubes()` 方法中可以添加更多可收集的立方体:

```javascript
createCubes() {
    this.cubes = [
        { x: 3, y: 1, z: 0, collected: false },
        { x: -4, y: 1.5, z: 5, collected: false },
        // 添加更多立方体...
        { x: 6, y: 2, z: -3, collected: false }
    ];
}
```

### 调整速度
修改 `updatePlayer()` 方法中的速度值:

```javascript
updatePlayer() {
    const speed = 0.1; // 增加这个值使移动更快
    
    // 其他代码保持不变...
}
```

### 调整跳跃高度
修改跳跃速度:

```javascript
if (e.code === 'Space' && !this.player.isJumping) {
    this.player.vy = 0.25; // 增加这个值跳得更高
    this.player.isJumping = true;
}
```

## 扩展建议

### 添加更多图形
- 使用纹理映射的3D模型
- 添加光照效果
- 实现阴影渲染
- 添加粒子效果

### 添加游戏机制
- 计分系统
- 多关卡设计
- 敌人AI
- 音效和背景音乐

### 性能优化
- 使用实例化渲染
- 实现视锥剔除
- 添加LOD(Level of Detail)系统

## 故障排除

### 黑屏
- 确保浏览器支持WebGL
- 尝试使用Chrome或Firefox最新版本
- 检查浏览器控制台的错误信息

### FPS很低
- 减少地形复杂度
- 降低渲染距离
- 关闭其他占用GPU的程序

### 控制无响应
- 点击页面确保焦点在游戏上
- 检查其他程序是否占用了WASD键

## 技术支持

### 兼容的浏览器
- Chrome 56+
- Firefox 51+
- Safari 10+
- Edge 79+

### WebGL要求
- WebGL 1.0或更高
- 硬件加速

## 许可证

本游戏演示仅供学习和参考目的。可以自由修改和扩展。
