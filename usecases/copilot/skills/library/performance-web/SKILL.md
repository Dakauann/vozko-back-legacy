---
name: performance-web
surface: site
description: Página rápida no celular: imagens leves, LCP, CLS e bloqueio de interação, fontes, bibliotecas e scripts de rastreamento; carregue quando site_look apontar lentidão ou antes de publicar uma página pesada.
---

# Performance web

Quem chega por anúncio está no celular, muitas vezes em 4G. Cada segundo a mais de carregamento derruba a conversão, e a Meta e o Google cobram mais por cliques que caem em páginas lentas.

## Métricas que o site_look mede (no navegador de quem edita, então são uma pista, não a nota do Google)
- LCP (o maior elemento visível aparecer): até 2,5 s. Aponta lcp_slow.
- CLS (a página pular enquanto carrega): até 0,1. Aponta cls_high.
- TBT (a página travada para toque): até 200 ms. Aponta tbt_high.
- Peso total: page_weight_high acima de cerca de 2,5 MB.

## Imagens (o maior ganho)
- O herói carrega primeiro: imagem do topo sem loading="lazy" e com fetchpriority="high"; as de baixo com loading="lazy" e decoding="async".
- Sempre width e height (ou uma proporção com aspect-*) para o espaço ficar reservado e a página não pular.
- Tamanho certo: uma foto de topo não precisa passar de cerca de 1600 px de largura; ícones são SVG (lucide).
- Vídeo de fundo no celular quase nunca compensa; use uma imagem.

## Fontes
- Duas famílias e poucos pesos. Texto com fonte do sistema como alternativa no tema (ui-sans-serif, system-ui) para aparecer na hora.

## Código
- Bibliotecas pesadas (three, gsap, lottie-web) só onde fazem diferença real, de preferência abaixo da dobra.
- Nada de animação de entrada atrasando o herói.
- Carrossel com muitas imagens grandes custa caro; prefira uma grade.

## Rastreamento
- Cada integração ligada em site_settings (pixel, GA4, GTM, Clarity) adiciona scripts. Ligue só as que o cliente usa de verdade; o banner de consentimento segura esses scripts até o visitante aceitar.

## Conferir
Rode site_look e leia os checks de desempenho; corrija o maior primeiro (quase sempre a imagem do topo) e olhe de novo.
