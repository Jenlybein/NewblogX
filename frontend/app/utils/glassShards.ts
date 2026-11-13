/**
 * 碎玻璃标题的几何数据（纯常量，不碰 DOM）。
 *
 * 来源：https://lanrhyme.top/ 首页 h1 的 Vue 组件 ParticleText
 * （压缩包里 scoped id `data-v-f276e628`）。九个多边形是它的原值，不要随手调：
 * 前六块（z=1）把整个文字盒铺满，拼起来才是完整字形；后三块（z=2）是浮在字上的
 * 玻璃片，位移/旋转幅度更大，也是"玻璃反光"那一层。
 *
 * 字段含义：
 *   id     —— 层的稳定标识，也是 rAF 里 sin/cos 抖动的相位；
 *   path   —— 相对文字盒的 `clip-path: polygon()` 百分比；
 *   z      —— 层深，1 = 拼图底片，2 = 浮层（带毛玻璃与边缘光）；
 *   speed  —— 视差灵敏度，浮层比底片更"跟手"；
 *   bx/by  —— 点击碎裂时朝哪个象限炸开（-1..1，0 = 不偏移）。
 */
export interface GlassShardDef {
  readonly id: number;
  readonly path: string;
  readonly z: 1 | 2;
  readonly speed: number;
  readonly bx: number;
  readonly by: number;
}

export const GLASS_SHARDS: readonly GlassShardDef[] = [
  { id: 1, path: "polygon(0% 0%, 35% 0%, 20% 60%, 0% 40%)", z: 1, speed: 0.02, bx: -1, by: -1 },
  { id: 2, path: "polygon(0% 40%, 20% 60%, 10% 100%, 0% 100%)", z: 1, speed: 0.03, bx: -1, by: 1 },
  { id: 3, path: "polygon(35% 0%, 65% 0%, 55% 45%, 20% 60%)", z: 1, speed: 0.015, bx: 0, by: -1 },
  { id: 4, path: "polygon(20% 60%, 55% 45%, 60% 100%, 10% 100%)", z: 1, speed: 0.025, bx: 0, by: 1 },
  {
    id: 5,
    path: "polygon(65% 0%, 100% 0%, 100% 50%, 85% 65%, 55% 45%)",
    z: 1,
    speed: 0.02,
    bx: 1,
    by: -1,
  },
  {
    id: 6,
    path: "polygon(55% 45%, 85% 65%, 100% 50%, 100% 100%, 60% 100%)",
    z: 1,
    speed: 0.035,
    bx: 1,
    by: 1,
  },
  { id: 7, path: "polygon(25% 20%, 45% 10%, 50% 40%, 30% 50%)", z: 2, speed: 0.06, bx: -0.5, by: -1 },
  { id: 8, path: "polygon(65% 40%, 85% 30%, 90% 70%, 70% 80%)", z: 2, speed: 0.08, bx: 1, by: 0 },
  { id: 9, path: "polygon(45% 60%, 60% 50%, 70% 80%, 50% 90%)", z: 2, speed: 0.05, bx: 0.5, by: 1 },
];
