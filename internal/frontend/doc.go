// Package frontend 将承载 Ebiten 演奏前端 (M2, PLAN.md §5.1)。
//
// 职责: 88 键渲染、按下动画 (力度→下沉/高亮)、延音踏板视觉、
// 二期 Synthesia 式分色下落音轨。前端只是总线的下游消费者,
// 不发声 (发声属 backend)、不产事件 (产事件属输入层)。
//
// 当前为 M0 占位。
package frontend
