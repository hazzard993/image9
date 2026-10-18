//go:build windows

package main

import "math"

type sourceColourSpace int

const (
	spaceSRGB sourceColourSpace = iota
	spaceP3
	spaceAdobeRGB
	spaceRec2020
)

func (v *viewer) setSourceColourSpace(space sourceColourSpace) {
	changed := v.sourceColourSpace != space
	v.sourceColourSpace = space
	for i, action := range v.colourSpaceActions {
		if action != nil {
			action.SetChecked(i == int(space))
		}
	}
	if changed && len(v.workspace) > 0 {
		v.reloadWorkspace()
	}
}

// Manual source interpretations, converted to 8-bit SDR sRGB for display and
// sampling. Matrices use D65 XYZ; these spaces share a white point, so no chromatic
// adaptation is necessary. This does not implement embedded ICC/monitor profiles.
// References: ICC RGB Encoding Registry and ITU-R BT.2020.
func convertSourceColourSpace(im *decodedImage, space sourceColourSpace, cancelled func() bool) error {
	if space == spaceSRGB {
		return nil
	}
	var xyz [3][3]float64
	switch space {
	case spaceP3:
		xyz = [3][3]float64{{0.4865709486, 0.2656676932, 0.1982172852}, {0.2289745641, 0.6917385218, 0.0792869141}, {0, 0.0451133819, 1.0439443689}}
	case spaceAdobeRGB:
		xyz = [3][3]float64{{0.5767309, 0.1855540, 0.1881852}, {0.2973769, 0.6273491, 0.0752741}, {0.0270343, 0.0706872, 0.9911085}}
	case spaceRec2020:
		xyz = [3][3]float64{{0.6369580483, 0.1446169036, 0.1688809752}, {0.2627002120, 0.6779980715, 0.0593017165}, {0, 0.0280726930, 1.0609850577}}
	default:
		return nil
	}
	xyzToSRGB := [3][3]float64{{3.2409699419, -1.5373831776, -0.4986107603}, {-0.9692436363, 1.8759675015, 0.0415550574}, {0.0556300797, -0.2039769589, 1.0569715142}}
	var matrix [3][3]float64
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			for k := 0; k < 3; k++ {
				matrix[i][j] += xyzToSRGB[i][k] * xyz[k][j]
			}
		}
	}
	var decode [256]float64
	for i := range decode {
		e := float64(i) / 255
		switch space {
		case spaceAdobeRGB:
			decode[i] = math.Pow(e, 563.0/256)
		case spaceRec2020:
			const alpha, beta = 1.09929682680944, 0.018053968510807
			if e < 4.5*beta {
				decode[i] = e / 4.5
			} else {
				decode[i] = math.Pow((e+alpha-1)/alpha, 1/0.45)
			}
		default:
			if e <= 0.04045 {
				decode[i] = e / 12.92
			} else {
				decode[i] = math.Pow((e+0.055)/1.055, 2.4)
			}
		}
	}
	var encode [65536]float64
	for i := range encode {
		linear := float64(i) / 65535
		if linear <= 0.0031308 {
			encode[i] = linear * 12.92
		} else {
			encode[i] = 1.055*math.Pow(linear, 1/2.4) - 0.055
		}
	}
	for y := 0; y < im.height; y++ {
		if cancelled() {
			return errLoadSuperseded
		}
		row := im.pixels[y*im.width*4 : (y+1)*im.width*4]
		for x := 0; x < len(row); x += 4 {
			alpha := int(row[x+3])
			if alpha == 0 {
				continue
			}
			unpremultiply := func(v byte) int { return min(255, (int(v)*255+alpha/2)/alpha) }
			r, g, b := decode[unpremultiply(row[x+2])], decode[unpremultiply(row[x+1])], decode[unpremultiply(row[x])]
			for c := 0; c < 3; c++ {
				linear := matrix[c][0]*r + matrix[c][1]*g + matrix[c][2]*b
				index := int(math.Round(math.Max(0, math.Min(1, linear)) * 65535))
				row[x+2-c] = byte(math.Round(encode[index] * float64(alpha)))
			}
		}
	}
	return nil
}
