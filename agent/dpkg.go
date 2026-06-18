package agent

import (
	"encoding/json"
	"fmt"

	"github.com/tadasv/go-dpkg"
	"k8s.io/klog/v2"
)

func (nm *NodeConfigManager) CheckPkgs() (diffMsg string, checkItems string, err error) {

	if len(nm.NodeGroup.Spec.PackageManagers) == 0 {
		klog.InfoS("zero.PackageManagers")
		return
	}
	packages, err := dpkg.ReadPackagesFromFile(nm.DpkgFileName)
	if err != nil {
		klog.ErrorS(err, "CheckPkgs.ReadPackagesFromFile.err",
			"DpkgFileName", nm.DpkgFileName,
		)
		return
	}

	desiredMap := map[string]string{}
	for _, onePkg := range nm.NodeGroup.Spec.PackageManagers {
		desiredMap[onePkg.Name] = onePkg.Version
	}

	checkItemsBs, _ := json.Marshal(desiredMap)
	checkItems = string(checkItemsBs)
	klog.InfoS("CheckPkgs.desiredMap.print", "desiredMap", desiredMap)
	actualMap := map[string]string{}
	for _, pkg := range packages {
		version, exists := desiredMap[pkg.Package]
		if exists {
			//klog.InfoS("CheckPkgs.Package.exists",
			//	"desiredMap", desiredMap,
			//	"pkg", pkg,
			//	"version", version,
			//)

			// 存在对比版本
			actualMap[pkg.Package] = pkg.Version
			if version != pkg.Version {
				diffMsg = fmt.Sprintf("%v\n[dpkg.version.missmatch][pkg=%v desired=%v actual=%v]",
					diffMsg,
					pkg.Package,
					version,
					pkg.Version,
				)
			}
		}
	}
	if len(actualMap) < len(desiredMap) {
		// 说明有包缺失
		// 把缺失的包过滤出来 ：遍历 desiredMap 不在	 actualMap就是缺失了
		for p, v := range desiredMap {
			_, exists := actualMap[p]
			if !exists {
				diffMsg = fmt.Sprintf("%v\n[dpkg.pkg.miss][pkg=%v desired=%v]",
					diffMsg,
					p,
					v,
				)
			}
		}
	}
	return
}
