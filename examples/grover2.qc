# 2-qubit Grover-like demonstration on a small teaching circuit.
# oracle marks |11> (CCZ implemented as H-CX-H around the target),
# then the diffusion operator; finally measure both qubits.
qubits 2
h 0
h 1
# oracle for |11>: CZ(0,1) = H1 CX(0,1) H1
h 1
cx 0,1
h 1
# diffusion: H X on both, CZ, X H on both
h 0
h 1
x 0
x 1
h 1
cx 0,1
h 1
x 0
x 1
h 0
h 1
m 0,1
