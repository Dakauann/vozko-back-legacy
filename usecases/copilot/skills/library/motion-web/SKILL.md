---
name: motion-web
surface: site
description: Movimento em páginas: entradas ao rolar, microinterações e efeitos de destaque que respeitam prefers-reduced-motion e nunca escondem texto sem JavaScript; carregue antes de animar uma página.
---

# Motion na web

Movimento serve para guiar o olho, dar resposta ao toque e criar um momento memorável. Animação que atrasa a leitura ou distrai da chamada atrapalha a conversão.

## Regra que não se quebra: o texto existe sem animação
- A página é pré-renderizada e lida por buscadores, por leitores de tela e por quem tem JavaScript lento. Conteúdo que começa com opacity-0 e só aparece por script fica invisível nesse HTML; o retorno aponta invisible_without_js e isso impede publicar.
- Faça a entrada partir de um estado visível ou use CSS que só esconde quando a animação vai rodar: animações com @keyframes e animation-fill-mode both, ou classes do Tailwind como motion-safe:animate-[...]. Com framer-motion, prefira whileInView com initial que não zera a opacidade do texto principal (um deslocamento curto e opacidade a partir de 0.6, por exemplo), ou anime só elementos decorativos.
- Herói: título, subtítulo e chamada aparecem imediatamente. Movimento no herói fica em fundo, imagem ou detalhe.

## prefers-reduced-motion
- Toda animação respeita a preferência do sistema: no Tailwind, motion-safe: para o que anima e motion-reduce: para desligar; no CSS, @media (prefers-reduced-motion: reduce); no framer-motion, useReducedMotion.
- Com movimento reduzido, troque deslocamentos e paralaxe por um fade curto ou nada.

## O que funciona
- Entradas ao rolar: deslocamento de 12 a 24 px com fade, 300 a 600 ms, easing suave de saída (cubic-bezier(0.22, 1, 0.36, 1)), em sequência curta dentro da seção (50 a 80 ms entre itens). Uma vez por elemento.
- Microinterações: botão que sobe 2 px e ganha sombra no hover (transition hover:-translate-y-0.5), foco visível animado, acordeão que abre com altura suave.
- Um momento de destaque por página: um número que conta até o valor, um carrossel de logos contínuo, um gradiente que se move devagar atrás do herói.
- Duração curta: interações em 150 a 250 ms, entradas em até 600 ms.

## O que evitar
- Animar width, height, top ou left em listas grandes; anime transform e opacity.
- Paralaxe pesado, vídeo de fundo no celular, rolagem sequestrada.
- Tudo entrando ao mesmo tempo, ou cada parágrafo com uma animação diferente.
- Carrossel automático com conteúdo importante (ninguém lê o segundo slide).

## Bibliotecas
- CSS e Tailwind resolvem a maior parte. framer-motion ou motion para entradas e gestos em React; gsap para sequências complexas; swiper ou embla para carrosséis; lottie-web para uma animação vetorial pronta.

## Conferir
site_look mostra a página depois de rolar uma vez até o fim: confira se as entradas aconteceram, se nada ficou invisível e se não há estouro horizontal no celular.
