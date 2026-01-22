# Memory comparison bar chart
# Usage: gnuplot -e "datafile='data.dat'; outfile='chart.png'" memory-comparison.gp

set terminal pngcairo size 800,500 enhanced font 'Arial,12'
set output outfile

set title "Peak Memory Usage Comparison" font 'Arial,14'
set xlabel "Repository"
set ylabel "Memory (MB)"

set style data histogram
set style histogram cluster gap 1
set style fill solid border -1
set boxwidth 0.9

set xtics rotate by -45
set key top left

set grid ytics

plot datafile using ($2/1024):xtic(1) title "gomdlint" linecolor rgb "#4CAF50", \
     '' using ($3/1024) title "markdownlint" linecolor rgb "#2196F3"
