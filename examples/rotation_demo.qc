# Parametrized gates; angles are in degrees.
qubits 3
rx(90) 0
ry(45) 1
p(180) 2
crx(30) 0,2
ccx 0,1,2
swap 1,2
measure
