---
name: direcao-de-arte-web
surface: site
description: Direção de arte de uma página: conceito, tipografia, paleta em tokens, ritmo, imagens e o que evita cara de modelo pronto; carregue antes de criar ou refazer o visual.
---

# Direção de arte web

Trate cada página como o trabalho de um estúdio que nunca entrega duas iguais. O cliente já recusou propostas genéricas; ele paga por um ponto de vista. Decida com intenção, a partir do negócio, do público e da oferta, e não do que costuma aparecer em modelos.

## Antes de desenhar
- Escreva para você, em uma frase, o conceito: a sensação que a página passa e por quê (por exemplo "consultório calmo e preciso, que tira o medo de dentista"). Toda escolha visual responde a essa frase.
- Junte o material real: logo, cores e fontes do sistema de design do workspace, fotos do cliente, uma referência que ele deu. Com uma referência, use site_capture_reference e leia os tokens medidos (cores, fontes, espaçamento); inspire-se no ritmo e na hierarquia, nunca copie textos, imagens ou a marca de outra empresa.
- Escolha uma ideia dominante e assuma: editorial com tipografia grande, cartões com profundidade, fotografia em sangria, blocos de cor chapada, grade assimétrica. Misturar quatro estilos é o que faz parecer modelo.

## Tipografia
- No máximo duas famílias: uma de display com personalidade para títulos e uma de texto muito legível. Declare no tema (--font-display e --font-sans) e use pelas classes font-display e font-sans.
- Escala com contraste de verdade: título principal entre text-5xl e text-7xl no computador, text-4xl no celular; corpo text-base a text-lg com leading-relaxed. text-balance em títulos, text-pretty em parágrafos.
- Linha de leitura curta: max-w-xl a max-w-2xl em parágrafos.
- Pesos com propósito: um peso forte para títulos, regular para texto. Itálico e caixa alta só como acento.

## Cor em tokens
- Toda cor nasce no tema como variável (--color-brand, --color-ink, --color-surface, --color-muted, --color-line, --color-accent) e entra na página por classes (bg-brand, text-ink). Nunca espalhe hex soltos nas seções; para mudar a paleta, set_token muda o tema e a página inteira acompanha.
- Uma cor de marca dominante, um acento raro para o que precisa chamar atenção (o botão principal), neutros com um leve tom da marca. oklch facilita gerar tons coerentes.
- Contraste mínimo de 4,5:1 para texto e 3:1 para texto grande; site_look aponta contrast_low. Fundo escuro pede texto quase branco, não cinza.

## Espaço e ritmo
- Respiro generoso e consistente: seções com py-20 a py-28 no computador e py-14 a py-16 no celular, conteúdo em max-w-6xl com px-6.
- Alterne a densidade: uma seção cheia, uma seção de respiro. Alterne fundos (surface, um tom de marca, escuro) para separar capítulos sem linhas.
- Alinhe tudo numa grade; quebre a grade uma vez, de propósito, onde quer chamar atenção.

## Imagens
- Fotos reais do cliente valem mais que qualquer ilustração. Sem fotos, gere com site_generate_image cenas específicas do negócio (o consultório, o produto em uso, a equipe trabalhando), coerentes com a paleta e com a mesma luz em todas.
- Texto dentro de uma imagem gerada é permitido quando faz sentido, mas o texto importante da página fica no HTML (é lido por buscadores e leitores de tela).
- Toda imagem tem alt que descreve o que mostra; decorativas ficam com alt vazio.
- Proporções consistentes (aspect-[4/3], aspect-video) e object-cover para não deformar.

## Profundidade e detalhe
- Detalhes pequenos fazem a página parecer feita à mão: um selo, um traço de destaque sob uma palavra, um gradiente suave atrás do herói, uma sombra colorida no botão principal.
- Arredondamento coerente em tudo, vindo de um token (--radius-card). Ajustes que o usuário vai querer mexer viram knobs em knobs.json (cor da marca, arredondamento, densidade).

## O que denuncia modelo pronto
- Herói com título centralizado genérico, três ícones em cartões iguais, depoimentos com foto de banco, gradiente roxo para azul sem relação com a marca.
- Emojis como ícones, sombras pesadas em tudo, cinco tamanhos de fonte diferentes, botões de estilos diferentes.
- Textos de enchimento. Se falta um dado real, deixe a lacuna visível e pergunte.

## Conferir
Depois de cada etapa grande, olhe com site_look no computador e no celular. Leia a imagem como o visitante: em três segundos dá para entender o que é, para quem é e o que fazer? A hierarquia guia o olho até o botão? Corrija com site_edit e olhe de novo.
