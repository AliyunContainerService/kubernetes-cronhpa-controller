#!/bin/bash
cd $(dirname $0)

wait_for_verify(){
while :
do
  AVAILABLE_NUM =`kubectl get deploy kubernetes-cronhpa-controller -n kube-system | grep kubernetes-cronhpa-controller |
  | wc -l`,$AVAILABLE_NUM
  if [ $AVAILABLE_NUM = 1 ];then
    break
  fi
done
}


while :
do
  read -a input
  if [ ${input[0]} == "helm"  -a ${input[1]}  == "install" -a  ${input[2]} == "kubernetes-cronhpa-controller-1.0.0" ]
  then
    sleep 3
    wait_for_verify()
  fi
done
  