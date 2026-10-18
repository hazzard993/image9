//go:build windows

package main

func (v *viewer) copyPathInInspectMode() {
	if v.highlightBounds || v.ctrlViewPinned {
		v.copyFocusedPath()
	}
}

func (v *viewer) handleCtrlC() {
	if v.ctrlViewPinned {
		v.copyFocusedPath()
	} else {
		v.clearWorkspace()
	}
}

func (v *viewer) handleH() {
	if v.ctrlViewPinned {
		v.navigate(-1)
	} else {
		v.toggleCtrlText()
	}
}

func (v *viewer) handleL() {
	if v.ctrlViewPinned {
		v.navigate(1)
	} else {
		v.showImageList()
	}
}

func (v *viewer) handleCtrlL() {
	if v.ctrlViewPinned {
		v.navigate(1)
	}
}

func (v *viewer) handleJ() {
	if v.ctrlViewPinned {
		v.navigateVertical(1)
	}
}

func (v *viewer) handleK() {
	if v.ctrlViewPinned {
		v.navigateVertical(-1)
	}
}
